package db_test

import (
	"database/sql"
	"testing"

	"gosilo/internal/db"
)

// testDB creates an in-memory SQLite database for testing.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}
