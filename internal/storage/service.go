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
	IfMatch     string   // unquoted ETag value
	IfNoneMatch []string // unquoted ETag values, or ["*"]
}

// IfNoneMatchStar returns true if If-None-Match is "*".
func (c Conditions) IfNoneMatchStar() bool {
	return len(c.IfNoneMatch) == 1 && c.IfNoneMatch[0] == "*"
}

// IfNoneMatchContains returns true if etag appears in the If-None-Match list.
func (c Conditions) IfNoneMatchContains(etag string) bool {
	for _, v := range c.IfNoneMatch {
		if v == etag {
			return true
		}
	}
	return false
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
	repo      *db.Repository
	blobs     blob.Store
	quota     *QuotaChecker
	egress *EgressTracker // optional — nil disables all egress tracking
	scanner   ContentScanner
}

// SetScanner sets the content scanner used to inspect uploads.
func (s *Service) SetScanner(sc ContentScanner) {
	s.scanner = sc
}

// SetEgressTracker installs the egress tracker used by GetDocument.
func (s *Service) SetEgressTracker(et *EgressTracker) { s.egress = et }

// NewService creates a new storage service.
func NewService(repo *db.Repository, blobs blob.Store, quota *QuotaChecker) *Service {
	return &Service{repo: repo, blobs: blobs, quota: quota}
}

// PutDocument stores a document.
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

		// Document/folder conflict: a document cannot be created at a path
		// that is already used as a folder prefix by other documents, or
		// where an ancestor segment matches an existing document.
		if err := txRepo.CheckPathConflict(ctx, userID, path); err != nil {
			if errors.Is(err, db.ErrPathConflict) {
				return ErrConflict
			}
			return err
		}

		// If-Match: require existing ETag to match.
		if cond.IfMatch != "" {
			if existing == nil || existing.ETag != cond.IfMatch {
				return ErrPreconditionFailed
			}
		}

		// If-None-Match: "*" means "only if the document doesn't exist yet".
		if cond.IfNoneMatchStar() && existing != nil {
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

		if _, err := txRepo.UpsertNode(ctx, userID, path, contentType, int64(len(data)), etag); err != nil {
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
	if node == nil {
		return nil, ErrNotFound
	}

	if cond.IfNoneMatchContains(node.ETag) {
		return nil, ErrNotModified
	}

	// Egress enforcement: consults User.EgressQuota (0 = unlimited) and the
	// global cap from settings (0 = disabled).
	if s.egress != nil {
		user, _ := s.repo.GetUserByID(ctx, userID)
		var userLimit int64
		if user != nil {
			userLimit = user.EgressQuota
		}
		if err := s.egress.CheckServe(ctx, userID, node.ContentLength, userLimit); err != nil {
			return nil, err
		}
	}

	content, err := s.blobs.Get(ctx, userID, path)
	if err != nil {
		return nil, err
	}

	// Record egress only after the blob is in-hand so an error on the
	// fetch path doesn't charge the user for bytes they won't receive.
	if s.egress != nil {
		s.egress.Record(userID, node.ContentLength)
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
	if node == nil {
		return nil, ErrNotFound
	}

	if cond.IfNoneMatchContains(node.ETag) {
		return nil, ErrNotModified
	}

	return &HeadResult{
		ContentType:   node.ContentType,
		ContentLength: node.ContentLength,
		ETag:          node.ETag,
	}, nil
}

// DeleteDocument removes a document.
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

// DeleteFolder recursively removes all documents under a folder path.
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

		// Delete all document nodes under this path.
		if err := txRepo.DeleteSubtree(ctx, userID, folderPath); err != nil {
			return err
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

// GetFolder returns a JSON-LD folder description with virtual children derived
// from document paths. Folders are implicit — they exist because documents
// exist beneath them. ETags are computed on-the-fly from children.
func (s *Service) GetFolder(ctx context.Context, userID int64, path string, cond Conditions) (*api.FolderDescription, string, error) {
	children, err := s.repo.ListVirtualChildren(ctx, userID, path)
	if err != nil {
		return nil, "", err
	}

	items := make(map[string]api.FolderItem)
	childETags := make(map[string]string)

	for _, child := range children {
		if child.IsFolder {
			// Virtual folder: ETag is computed from descendant documents.
			items[child.Name] = api.FolderItem{ETag: child.ETag}
		} else {
			cl := child.ContentLength
			items[child.Name] = api.FolderItem{
				ETag:          child.ETag,
				ContentType:   child.ContentType,
				ContentLength: &cl,
				LastModified:  child.UpdatedAt.UTC().Format(http.TimeFormat),
			}
		}
		childETags[child.Name] = child.ETag
	}

	etag := FolderETag(childETags)

	if cond.IfNoneMatchContains(etag) {
		return nil, etag, ErrNotModified
	}

	desc := &api.FolderDescription{
		Context: "http://remotestorage.io/spec/folder-description",
		Items:   items,
	}

	return desc, etag, nil
}
