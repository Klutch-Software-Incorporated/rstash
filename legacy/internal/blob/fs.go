package blob

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// FSStore stores blobs as files on the filesystem.
type FSStore struct {
	basePath string
}

// NewFSStore creates a new FSStore, ensuring the base directory exists.
func NewFSStore(basePath string) (*FSStore, error) {
	abs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("resolve blob path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("create blob directory: %w", err)
	}
	return &FSStore{basePath: abs}, nil
}

// blobPath returns the filesystem path for a given user/storage path.
// It validates the result stays under basePath to prevent directory traversal.
func (s *FSStore) blobPath(userID int64, path string) (string, error) {
	rel := filepath.Join(strconv.FormatInt(userID, 10), filepath.FromSlash(path))
	full := filepath.Join(s.basePath, rel)
	// Clean and verify the path stays under basePath.
	full = filepath.Clean(full)
	if !strings.HasPrefix(full, s.basePath+string(filepath.Separator)) && full != s.basePath {
		return "", fmt.Errorf("path traversal rejected: %s", path)
	}
	return full, nil
}

// Get retrieves a blob from the filesystem.
func (s *FSStore) Get(_ context.Context, userID int64, path string) (io.ReadCloser, error) {
	p, err := s.blobPath(userID, path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	return f, nil
}

// Put stores a blob on the filesystem using atomic rename.
func (s *FSStore) Put(_ context.Context, userID int64, path string, data []byte) error {
	p, err := s.blobPath(userID, path)
	if err != nil {
		return err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create blob directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".blob-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write blob: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename blob: %w", err)
	}
	return nil
}

// Delete removes a blob from the filesystem.
func (s *FSStore) Delete(_ context.Context, userID int64, path string) error {
	p, err := s.blobPath(userID, path)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

// DeleteTree removes all blobs under a folder path by removing the directory.
func (s *FSStore) DeleteTree(_ context.Context, userID int64, folderPath string) error {
	p, err := s.blobPath(userID, folderPath)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(p); err != nil {
		return fmt.Errorf("delete blob tree: %w", err)
	}
	return nil
}

// Count returns the total number of blob files stored.
func (s *FSStore) Count(_ context.Context) (int64, error) {
	var n int64
	err := filepath.Walk(s.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			n++
		}
		return nil
	})
	return n, err
}

// Close is a no-op for the filesystem backend.
func (s *FSStore) Close() error {
	return nil
}
