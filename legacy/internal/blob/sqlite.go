package blob

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"

	_ "github.com/glebarez/go-sqlite"
)

const blobSchema = `
CREATE TABLE IF NOT EXISTS blobs (
    user_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    data BLOB NOT NULL,
    PRIMARY KEY(user_id, path)
);
`

// SQLiteStore stores blobs in a dedicated SQLite database file.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens a SQLite database at the given path and ensures
// the blobs table exists. The store owns the connection and must be closed.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open blob database: %w", err)
	}

	// SQLite is single-writer; limit pool to 1 connection.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL mode on blob database: %w", err)
	}

	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set busy_timeout on blob database: %w", err)
	}

	// Enable case-sensitive LIKE (SQLite default is case-insensitive for ASCII).
	if _, err := db.Exec("PRAGMA case_sensitive_like=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable case-sensitive LIKE on blob database: %w", err)
	}

	if _, err := db.Exec(blobSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create blobs table: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// Get retrieves a blob.
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

// Put stores a blob (insert or replace).
func (s *SQLiteStore) Put(ctx context.Context, userID int64, path string, data []byte) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO blobs (user_id, path, data) VALUES (?, ?, ?) ON CONFLICT(user_id, path) DO UPDATE SET data = excluded.data",
		userID, path, data,
	)
	if err != nil {
		return fmt.Errorf("put blob: %w", err)
	}
	return nil
}

// Delete removes a blob.
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

// DeleteTree removes all blobs under a folder path prefix.
func (s *SQLiteStore) DeleteTree(ctx context.Context, userID int64, folderPath string) error {
	_, err := s.db.ExecContext(ctx,
		"DELETE FROM blobs WHERE user_id = ? AND path LIKE ? || '%'",
		userID, folderPath,
	)
	if err != nil {
		return fmt.Errorf("delete blob tree: %w", err)
	}
	return nil
}

// Count returns the total number of blobs stored.
func (s *SQLiteStore) Count(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM blobs").Scan(&n)
	return n, err
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
