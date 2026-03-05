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

// Counter is an optional interface that blob stores can implement to report
// the total number of stored blobs. Used by the doctor command for
// consistency checking against metadata node counts.
type Counter interface {
	Count(ctx context.Context) (int64, error)
}
