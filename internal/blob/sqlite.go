package blob

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
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
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if _, err := db.Exec(blobSchema); err != nil {
		return nil, fmt.Errorf("create blobs table: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Get(ctx context.Context, userID int64, path string) (io.ReadCloser, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx,
		"SELECT data FROM blobs WHERE user_id = ? AND path = ?",
		userID, path,
	).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *SQLiteStore) Put(ctx context.Context, userID int64, path string, content io.Reader) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("read content: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		"INSERT INTO blobs (user_id, path, data) VALUES (?, ?, ?) ON CONFLICT(user_id, path) DO UPDATE SET data = excluded.data",
		userID, path, data,
	)
	if err != nil {
		return fmt.Errorf("put blob: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Delete(ctx context.Context, userID int64, path string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM blobs WHERE user_id = ? AND path = ?",
		userID, path,
	)
	if err != nil {
		return fmt.Errorf("delete blob: %w", err)
	}
	return nil
}
