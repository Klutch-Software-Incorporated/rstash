package blob

import (
	"context"
	"io"
)

// Store is the interface for blob storage backends.
type Store interface {
	Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error)
	Put(ctx context.Context, userID int64, path string, data []byte) error
	Delete(ctx context.Context, userID int64, path string) error
	DeleteTree(ctx context.Context, userID int64, folderPath string) error
	Close() error
}
