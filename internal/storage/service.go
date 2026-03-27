package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"rstash/api"
	"rstash/internal/blob"
	"rstash/internal/db"
)

// Sentinel errors returned by service methods.
var (
	ErrNotFound           = errors.New("not found")
	ErrPreconditionFailed = errors.New("precondition failed")
	ErrConflict           = errors.New("conflict")
	ErrNotModified        = errors.New("not modified")
	ErrPayloadTooLarge    = errors.New("payload too large")
	ErrContentRejected    = errors.New("content rejected")
)

// Conditions holds parsed If-Match / If-None-Match header values (unquoted).
type Conditions struct {
	IfMatch     string // unquoted ETag value
	IfNoneMatch string // unquoted ETag value, or "*"
}

// PutResult is returned by PutDocument.
type PutResult struct {
	ETag  string
	IsNew bool
}

// GetResult is returned by GetDocument.
type GetResult struct {
	Content       io.ReadCloser
	ContentType   string
	ContentLength int64
	ETag          string
}

// HeadResult is returned by HeadDocument (metadata only, no blob fetch).
type HeadResult struct {
	ContentType   string
	ContentLength int64
	ETag          string
}

// DeleteResult is returned by DeleteDocument.
type DeleteResult struct {
	ETag string // ETag of the deleted version
}

// Service orchestrates storage operations (blob + node + ETags).
type Service struct {
	repo    *db.Repository
	blobs   blob.Store
	quota   *QuotaChecker
	scanner ContentScanner
}

// SetScanner sets the content scanner used to inspect uploads.
func (s *Service) SetScanner(sc ContentScanner) {
	s.scanner = sc
}

// NewService creates a new storage service.
func NewService(repo *db.Repository, blobs blob.Store, quota *QuotaChecker) *Service {
	return &Service{repo: repo, blobs: blobs, quota: quota}
}

// PutDocument stores a document and propagates folder ETags up to root.
// Blob is written first (non-transactional), then metadata is updated in a TX.
func (s *Service) PutDocument(ctx context.Context, userID int64, path string, content io.Reader, contentType string, cond Conditions) (*PutResult, error) {
	if strings.HasSuffix(path, "/") {
		return nil, ErrConflict
	}

	data, err := io.ReadAll(content)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return nil, ErrPayloadTooLarge
		}
		return nil, fmt.Errorf("read content: %w", err)
	}

	// Run content scanner if configured.
	if s.scanner != nil {
		result := s.scanner.Scan(ctx, data, contentType, userID, path)
		if !result.Allowed {
			return nil, fmt.Errorf("%w: %s", ErrContentRejected, result.Reason)
		}
	}

	etag := DocumentETag(data)

	if s.quota != nil {
		s.quota.Lock()
		defer s.quota.Unlock()
	}

	// Write blob first (non-transactional).
	if err := s.blobs.Put(ctx, userID, path, data); err != nil {
		return nil, err
	}

	// Now update metadata in a transaction.
	var putResult *PutResult
	txErr := s.repo.Transaction(func(txRepo *db.Repository) error {
		existing, err := txRepo.GetNode(ctx, userID, path)
		if err != nil {
			return err
		}

		// Document/folder conflict: existing node at this path is a folder.
		if existing != nil && existing.IsFolder {
			return ErrConflict
		}

		// If-Match: require existing ETag to match.
		if cond.IfMatch != "" {
			if existing == nil || existing.ETag != cond.IfMatch {
				return ErrPreconditionFailed
			}
		}

		// If-None-Match: "*" means "only if the document doesn't exist yet".
		if cond.IfNoneMatch == "*" && existing != nil {
			return ErrPreconditionFailed
		}

		// Quota check: compute net delta and verify.
		if s.quota != nil {
			oldSize := int64(0)
			if existing != nil {
				oldSize = existing.ContentLength
			}
			if err := s.quota.Check(ctx, txRepo, userID, int64(len(data))-oldSize); err != nil {
				return err
			}
		}

		isNew := existing == nil

		if _, err := txRepo.UpsertNode(ctx, userID, path, false, contentType, int64(len(data)), etag); err != nil {
			return err
		}

		if err := propagateETags(ctx, txRepo, userID, path); err != nil {
			return err
		}

		putResult = &PutResult{ETag: etag, IsNew: isNew}
		return nil
	})

	if txErr != nil {
		// Best-effort blob cleanup on metadata TX failure for non-service errors.
		if !errors.Is(txErr, ErrConflict) && !errors.Is(txErr, ErrPreconditionFailed) {
			if cleanErr := s.blobs.Delete(ctx, userID, path); cleanErr != nil {
				slog.Warn("failed to clean up orphaned blob after TX failure", "path", path, "error", cleanErr)
			}
		}
		return nil, txErr
	}

	return putResult, nil
}

// GetDocument retrieves a document's content and metadata.
func (s *Service) GetDocument(ctx context.Context, userID int64, path string, cond Conditions) (*GetResult, error) {
	node, err := s.repo.GetNode(ctx, userID, path)
	if err != nil {
		return nil, err
	}
	if node == nil || node.IsFolder {
		return nil, ErrNotFound
	}

	if cond.IfNoneMatch != "" && node.ETag == cond.IfNoneMatch {
		return nil, ErrNotModified
	}

	content, err := s.blobs.Get(ctx, userID, path)
	if err != nil {
		return nil, err
	}

	return &GetResult{
		Content:       content,
		ContentType:   node.ContentType,
		ContentLength: node.ContentLength,
		ETag:          node.ETag,
	}, nil
}

// HeadDocument returns document metadata without fetching the blob.
func (s *Service) HeadDocument(ctx context.Context, userID int64, path string, cond Conditions) (*HeadResult, error) {
	node, err := s.repo.GetNode(ctx, userID, path)
	if err != nil {
		return nil, err
	}
	if node == nil || node.IsFolder {
		return nil, ErrNotFound
	}

	if cond.IfNoneMatch != "" && node.ETag == cond.IfNoneMatch {
		return nil, ErrNotModified
	}

	return &HeadResult{
		ContentType:   node.ContentType,
		ContentLength: node.ContentLength,
		ETag:          node.ETag,
	}, nil
}

// DeleteDocument removes a document and propagates folder ETags, cleaning up
// empty ancestor folders (except root "/").
// Metadata is deleted in a TX first, then the blob is removed after commit.
func (s *Service) DeleteDocument(ctx context.Context, userID int64, path string, cond Conditions) (*DeleteResult, error) {
	var deleteResult *DeleteResult

	txErr := s.repo.Transaction(func(txRepo *db.Repository) error {
		node, err := txRepo.GetNode(ctx, userID, path)
		if err != nil {
			return err
		}
		if node == nil {
			return ErrNotFound
		}

		if cond.IfMatch != "" && node.ETag != cond.IfMatch {
			return ErrPreconditionFailed
		}

		deletedETag := node.ETag

		if err := txRepo.DeleteNode(ctx, userID, path); err != nil {
			return err
		}

		// Propagate folder ETags and clean up empty ancestor folders.
		ancestors := db.AncestorPaths(path)
		for _, folderPath := range ancestors {
			children, err := txRepo.ListChildren(ctx, userID, folderPath)
			if err != nil {
				return err
			}

			if len(children) == 0 && folderPath != "/" {
				if err := txRepo.DeleteNode(ctx, userID, folderPath); err != nil {
					return err
				}
			} else {
				childETags := make(map[string]string)
				for _, child := range children {
					name := strings.TrimPrefix(child.Path, folderPath)
					childETags[name] = child.ETag
				}
				etag := FolderETag(childETags)
				if _, err := txRepo.UpsertNode(ctx, userID, folderPath, true, "", 0, etag); err != nil {
					return err
				}
			}
		}

		deleteResult = &DeleteResult{ETag: deletedETag}
		return nil
	})

	if txErr != nil {
		return nil, txErr
	}

	// Delete blob after successful metadata commit (best-effort).
	if err := s.blobs.Delete(ctx, userID, path); err != nil {
		slog.Warn("failed to delete blob after metadata commit", "path", path, "error", err)
	}

	return deleteResult, nil
}

// DeleteFolder recursively removes a folder and all its contents (files and
// subfolders), then propagates ETags up to ancestor folders.
// Metadata is deleted in a TX first, then blobs are removed after commit.
func (s *Service) DeleteFolder(ctx context.Context, userID int64, folderPath string) (int, error) {
	if !strings.HasSuffix(folderPath, "/") {
		return 0, ErrConflict
	}
	if folderPath == "/" {
		return 0, ErrConflict // cannot delete root
	}

	var fileCount int

	txErr := s.repo.Transaction(func(txRepo *db.Repository) error {
		// Count files being deleted (for feedback).
		files, err := txRepo.ListDescendantFiles(ctx, userID, folderPath)
		if err != nil {
			return err
		}
		fileCount = len(files)

		// Delete all nodes (files + folders) under this path, including the folder itself.
		if err := txRepo.DeleteSubtree(ctx, userID, folderPath); err != nil {
			return err
		}

		// Propagate ETags and clean up empty ancestor folders.
		ancestors := db.AncestorPaths(folderPath)
		for _, ancestorPath := range ancestors {
			children, err := txRepo.ListChildren(ctx, userID, ancestorPath)
			if err != nil {
				return err
			}

			if len(children) == 0 && ancestorPath != "/" {
				if err := txRepo.DeleteNode(ctx, userID, ancestorPath); err != nil {
					return err
				}
			} else {
				childETags := make(map[string]string)
				for _, child := range children {
					name := strings.TrimPrefix(child.Path, ancestorPath)
					childETags[name] = child.ETag
				}
				etag := FolderETag(childETags)
				if _, err := txRepo.UpsertNode(ctx, userID, ancestorPath, true, "", 0, etag); err != nil {
					return err
				}
			}
		}

		return nil
	})

	if txErr != nil {
		return 0, txErr
	}

	// Delete blobs after successful metadata commit (best-effort).
	if err := s.blobs.DeleteTree(ctx, userID, folderPath); err != nil {
		slog.Warn("failed to delete blob tree after metadata commit", "folder", folderPath, "error", err)
	}

	return fileCount, nil
}

// GetFolder returns a JSON-LD folder description listing direct children.
func (s *Service) GetFolder(ctx context.Context, userID int64, path string, cond Conditions) (*api.FolderDescription, string, error) {
	node, err := s.repo.GetNode(ctx, userID, path)
	if err != nil {
		return nil, "", err
	}

	var etag string
	if node != nil {
		etag = node.ETag
	}

	if cond.IfNoneMatch != "" && etag != "" && etag == cond.IfNoneMatch {
		return nil, etag, ErrNotModified
	}

	children, err := s.repo.ListChildren(ctx, userID, path)
	if err != nil {
		return nil, "", err
	}

	items := make(map[string]api.FolderItem)
	for _, child := range children {
		name := strings.TrimPrefix(child.Path, path)
		item := api.FolderItem{ETag: child.ETag}
		if !child.IsFolder {
			item.ContentType = child.ContentType
			cl := child.ContentLength
			item.ContentLength = &cl
			lastMod := child.UpdatedAt.Unix()
			item.LastModified = &lastMod
		}
		items[name] = item
	}

	// Compute ETag from children if no folder node exists (e.g., empty root).
	if etag == "" {
		childETags := make(map[string]string)
		for name, item := range items {
			childETags[name] = item.ETag
		}
		etag = FolderETag(childETags)
	}

	desc := &api.FolderDescription{
		Context: "http://remotestorage.io/spec/folder-description",
		Items:   items,
	}

	return desc, etag, nil
}

// CreateFolder creates an empty folder node and propagates ETags.
// The path must end with "/" and must not be root "/".
func (s *Service) CreateFolder(ctx context.Context, userID int64, folderPath string) error {
	if !strings.HasSuffix(folderPath, "/") {
		return ErrConflict
	}
	if folderPath == "/" {
		return ErrConflict
	}

	return s.repo.Transaction(func(txRepo *db.Repository) error {
		existing, err := txRepo.GetNode(ctx, userID, folderPath)
		if err != nil {
			return err
		}
		if existing != nil {
			return ErrConflict
		}

		etag := FolderETag(nil)
		if _, err := txRepo.UpsertNode(ctx, userID, folderPath, true, "", 0, etag); err != nil {
			return err
		}

		return propagateETags(ctx, txRepo, userID, folderPath)
	})
}

// propagateETags recomputes folder ETags for all ancestors of path.
func propagateETags(ctx context.Context, repo *db.Repository, userID int64, path string) error {
	ancestors := db.AncestorPaths(path)
	for _, folderPath := range ancestors {
		children, err := repo.ListChildren(ctx, userID, folderPath)
		if err != nil {
			return err
		}

		childETags := make(map[string]string)
		for _, child := range children {
			name := strings.TrimPrefix(child.Path, folderPath)
			childETags[name] = child.ETag
		}

		etag := FolderETag(childETags)
		if _, err := repo.UpsertNode(ctx, userID, folderPath, true, "", 0, etag); err != nil {
			return err
		}
	}
	return nil
}
