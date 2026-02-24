package storage

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"

	"gosilo/api"
	"gosilo/internal/blob"
	"gosilo/internal/db"
)

// Sentinel errors returned by service methods.
var (
	ErrNotFound           = errors.New("not found")
	ErrPreconditionFailed = errors.New("precondition failed")
	ErrConflict           = errors.New("conflict")
	ErrNotModified        = errors.New("not modified")
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

// Service orchestrates storage operations (blob + node + ETags) in transactions.
type Service struct {
	database *sql.DB
	blobs    *blob.SQLiteStore
}

// NewService creates a new storage service.
func NewService(database *sql.DB, blobs *blob.SQLiteStore) *Service {
	return &Service{database: database, blobs: blobs}
}

// PutDocument stores a document and propagates folder ETags up to root.
func (s *Service) PutDocument(ctx context.Context, userID int64, path string, content io.Reader, contentType string, cond Conditions) (*PutResult, error) {
	if strings.HasSuffix(path, "/") {
		return nil, ErrConflict
	}

	data, err := io.ReadAll(content)
	if err != nil {
		return nil, fmt.Errorf("read content: %w", err)
	}

	etag := DocumentETag(data)

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	existing, err := db.GetNode(ctx, tx, userID, path)
	if err != nil {
		return nil, err
	}

	// Document/folder conflict: existing node at this path is a folder.
	if existing != nil && existing.IsFolder {
		return nil, ErrConflict
	}

	// If-Match: require existing ETag to match.
	if cond.IfMatch != "" {
		if existing == nil || existing.ETag != cond.IfMatch {
			return nil, ErrPreconditionFailed
		}
	}

	// If-None-Match: "*" means "only if the document doesn't exist yet".
	if cond.IfNoneMatch == "*" && existing != nil {
		return nil, ErrPreconditionFailed
	}

	isNew := existing == nil

	if err := s.blobs.PutWith(ctx, tx, userID, path, bytes.NewReader(data)); err != nil {
		return nil, err
	}

	if _, err := db.UpsertNode(ctx, tx, userID, path, false, contentType, int64(len(data)), etag); err != nil {
		return nil, err
	}

	if err := s.propagateETags(ctx, tx, userID, path); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &PutResult{ETag: etag, IsNew: isNew}, nil
}

// GetDocument retrieves a document's content and metadata.
func (s *Service) GetDocument(ctx context.Context, userID int64, path string, cond Conditions) (*GetResult, error) {
	node, err := db.GetNode(ctx, s.database, userID, path)
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
	node, err := db.GetNode(ctx, s.database, userID, path)
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
func (s *Service) DeleteDocument(ctx context.Context, userID int64, path string, cond Conditions) (*DeleteResult, error) {
	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	node, err := db.GetNode(ctx, tx, userID, path)
	if err != nil {
		return nil, err
	}
	if node == nil {
		return nil, ErrNotFound
	}

	if cond.IfMatch != "" && node.ETag != cond.IfMatch {
		return nil, ErrPreconditionFailed
	}

	deletedETag := node.ETag

	if err := s.blobs.DeleteWith(ctx, tx, userID, path); err != nil {
		return nil, err
	}
	if err := db.DeleteNode(ctx, tx, userID, path); err != nil {
		return nil, err
	}

	// Propagate folder ETags and clean up empty ancestor folders.
	ancestors := db.AncestorPaths(path)
	for _, folderPath := range ancestors {
		children, err := db.ListChildren(ctx, tx, userID, folderPath)
		if err != nil {
			return nil, err
		}

		if len(children) == 0 && folderPath != "/" {
			if err := db.DeleteNode(ctx, tx, userID, folderPath); err != nil {
				return nil, err
			}
		} else {
			childETags := make(map[string]string)
			for _, child := range children {
				name := strings.TrimPrefix(child.Path, folderPath)
				childETags[name] = child.ETag
			}
			etag := FolderETag(childETags)
			if _, err := db.UpsertNode(ctx, tx, userID, folderPath, true, "", 0, etag); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &DeleteResult{ETag: deletedETag}, nil
}

// DeleteFolder recursively removes a folder and all its contents (files and
// subfolders), then propagates ETags up to ancestor folders.
func (s *Service) DeleteFolder(ctx context.Context, userID int64, folderPath string) (int, error) {
	if !strings.HasSuffix(folderPath, "/") {
		return 0, ErrConflict
	}
	if folderPath == "/" {
		return 0, ErrConflict // cannot delete root
	}

	tx, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Count files being deleted (for feedback).
	files, err := db.ListDescendantFiles(ctx, tx, userID, folderPath)
	if err != nil {
		return 0, err
	}
	fileCount := len(files)

	// Delete all blobs under this folder.
	if err := s.blobs.DeleteTreeWith(ctx, tx, userID, folderPath); err != nil {
		return 0, err
	}

	// Delete all nodes (files + folders) under this path, including the folder itself.
	if err := db.DeleteSubtree(ctx, tx, userID, folderPath); err != nil {
		return 0, err
	}

	// Propagate ETags and clean up empty ancestor folders.
	ancestors := db.AncestorPaths(folderPath)
	for _, ancestorPath := range ancestors {
		children, err := db.ListChildren(ctx, tx, userID, ancestorPath)
		if err != nil {
			return 0, err
		}

		if len(children) == 0 && ancestorPath != "/" {
			if err := db.DeleteNode(ctx, tx, userID, ancestorPath); err != nil {
				return 0, err
			}
		} else {
			childETags := make(map[string]string)
			for _, child := range children {
				name := strings.TrimPrefix(child.Path, ancestorPath)
				childETags[name] = child.ETag
			}
			etag := FolderETag(childETags)
			if _, err := db.UpsertNode(ctx, tx, userID, ancestorPath, true, "", 0, etag); err != nil {
				return 0, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}

	return fileCount, nil
}

// GetFolder returns a JSON-LD folder description listing direct children.
func (s *Service) GetFolder(ctx context.Context, userID int64, path string, cond Conditions) (*api.FolderDescription, string, error) {
	node, err := db.GetNode(ctx, s.database, userID, path)
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

	children, err := db.ListChildren(ctx, s.database, userID, path)
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

// propagateETags recomputes folder ETags for all ancestors of path.
func (s *Service) propagateETags(ctx context.Context, tx *sql.Tx, userID int64, path string) error {
	ancestors := db.AncestorPaths(path)
	for _, folderPath := range ancestors {
		children, err := db.ListChildren(ctx, tx, userID, folderPath)
		if err != nil {
			return err
		}

		childETags := make(map[string]string)
		for _, child := range children {
			name := strings.TrimPrefix(child.Path, folderPath)
			childETags[name] = child.ETag
		}

		etag := FolderETag(childETags)
		if _, err := db.UpsertNode(ctx, tx, userID, folderPath, true, "", 0, etag); err != nil {
			return err
		}
	}
	return nil
}
