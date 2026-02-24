package blob

import (
	"context"
	"io"

	"gosilo/internal/db"
)

// Store is the interface for blob storage backends.
type Store interface {
	Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error)
	Put(ctx context.Context, userID int64, path string, content io.Reader) error
	Delete(ctx context.Context, userID int64, path string) error
	GetWith(ctx context.Context, q db.Querier, userID int64, path string) (io.ReadCloser, error)
	PutWith(ctx context.Context, q db.Querier, userID int64, path string, content io.Reader) error
	DeleteWith(ctx context.Context, q db.Querier, userID int64, path string) error
	DeleteTreeWith(ctx context.Context, q db.Querier, userID int64, folderPath string) error
}
