package blob

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"

	"gosilo/internal/db"
)

const blobSchema = `
CREATE TABLE IF NOT EXISTS blobs (
    user_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    data BLOB NOT NULL,
    PRIMARY KEY(user_id, path)
);
`

// SQLiteStore stores blobs in the SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLiteStore and ensures the blobs table exists.
func NewSQLiteStore(database *sql.DB) (*SQLiteStore, error) {
	if _, err := database.Exec(blobSchema); err != nil {
		return nil, fmt.Errorf("create blobs table: %w", err)
	}
	return &SQLiteStore{db: database}, nil
}

// GetWith retrieves a blob using the provided Querier (supports transactions).
func (s *SQLiteStore) GetWith(ctx context.Context, q db.Querier, userID int64, path string) (io.ReadCloser, error) {
	var data []byte
	err := q.QueryRowContext(ctx,
		"SELECT data FROM blobs WHERE user_id = ? AND path = ?",
		userID, path,
	).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// Get retrieves a blob using the store's database connection.
func (s *SQLiteStore) Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error) {
	return s.GetWith(ctx, s.db, userID, path)
}

// PutWith stores a blob using the provided Querier (supports transactions).
func (s *SQLiteStore) PutWith(ctx context.Context, q db.Querier, userID int64, path string, content io.Reader) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}
	_, err = q.ExecContext(ctx,
		"INSERT INTO blobs (user_id, path, data) VALUES (?, ?, ?) ON CONFLICT(user_id, path) DO UPDATE SET data = excluded.data",
		userID, path, data,
	)
	if err != nil {
		return fmt.Errorf("put blob: %w", err)
	}
	return nil
}

// Put stores a blob using the store's database connection.
func (s *SQLiteStore) Put(ctx context.Context, userID int64, path string, content io.Reader) error {
	return s.PutWith(ctx, s.db, userID, path, content)
}

// DeleteWith removes a blob using the provided Querier (supports transactions).
func (s *SQLiteStore) DeleteWith(ctx context.Context, q db.Querier, userID int64, path string) error {
	_, err := q.ExecContext(ctx,
		"DELETE FROM blobs WHERE user_id = ? AND path = ?",
		userID, path,
	)
	if err != nil {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}

// Delete removes a blob using the store's database connection.
func (s *SQLiteStore) Delete(ctx context.Context, userID int64, path string) error {
	return s.DeleteWith(ctx, s.db, userID, path)
}

// DeleteTreeWith removes all blobs under a folder path prefix using the provided
// Querier (supports transactions).
func (s *SQLiteStore) DeleteTreeWith(ctx context.Context, q db.Querier, userID int64, folderPath string) error {
	_, err := q.ExecContext(ctx,
		"DELETE FROM blobs WHERE user_id = ? AND path LIKE ? || '%'",
		userID, folderPath,
	)
	if err != nil {
		return fmt.Errorf("delete blob tree: %w", err)
	}
	return nil
}
