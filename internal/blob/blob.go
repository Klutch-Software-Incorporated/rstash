package blob

import (
	"context"
	"io"
)

// Store is the interface for blob storage backends.
type Store interface {
	Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error)
	Put(ctx context.Context, userID int64, path string, content io.Reader) error
	Delete(ctx context.Context, userID int64, path string) error
}
