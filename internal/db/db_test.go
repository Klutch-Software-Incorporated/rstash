package db_test

import (
	"testing"

	"gosilo/internal/db"
)

// testDB creates an in-memory SQLite database for testing.
func testDB(t *testing.T) *db.Repository {
	t.Helper()
	repo, err := db.OpenRepository("sqlite::memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { repo.Close() })
	return repo
}
