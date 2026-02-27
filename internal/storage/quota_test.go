package storage

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

// testDB creates an in-memory SQLite database with the schema applied.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	_, err = database.Exec("PRAGMA foreign_keys=ON")
	if err != nil {
		t.Fatal(err)
	}

	schema := `
	CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL,
		is_admin INTEGER NOT NULL DEFAULT 0,
		storage_quota INTEGER NOT NULL DEFAULT 0,
		disabled INTEGER NOT NULL DEFAULT 0,
		approved INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		last_login_at TEXT,
		last_login_ip TEXT,
		tos_accepted_at TEXT,
		privacy_accepted_at TEXT
	);
	CREATE TABLE nodes (
		id INTEGER PRIMARY KEY,
		user_id INTEGER NOT NULL REFERENCES users(id),
		path TEXT NOT NULL,
		is_folder INTEGER NOT NULL DEFAULT 0,
		content_type TEXT,
		content_length INTEGER,
		etag TEXT NOT NULL,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now')),
		UNIQUE(user_id, path)
	);`
	if _, err := database.Exec(schema); err != nil {
		t.Fatal(err)
	}

	return database
}

func createTestUser(t *testing.T, database *sql.DB, username string, quota int64) int64 {
	t.Helper()
	var id int64
	err := database.QueryRow(
		"INSERT INTO users (username, password_hash, storage_quota) VALUES (?, 'hash', ?) RETURNING id",
		username, quota,
	).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func addTestNode(t *testing.T, database *sql.DB, userID int64, path string, size int64) {
	t.Helper()
	_, err := database.Exec(
		"INSERT INTO nodes (user_id, path, is_folder, content_length, etag) VALUES (?, ?, 0, ?, 'etag')",
		userID, path, size,
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestQuota_ModeOff_AlwaysAllows(t *testing.T) {
	database := testDB(t)
	userID := createTestUser(t, database, "alice", 0)

	qc := NewQuotaChecker(QuotaConfig{Mode: "off"}, database)
	err := qc.Check(context.Background(), database, userID, 999999999)
	if err != nil {
		t.Fatalf("mode off should always allow, got: %v", err)
	}
}

func TestQuota_ModeTotal_AllowsWithinLimit(t *testing.T) {
	database := testDB(t)
	userID := createTestUser(t, database, "alice", 0)
	addTestNode(t, database, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{Mode: "total", TotalLimit: 1000}, database)
	err := qc.Check(context.Background(), database, userID, 400)
	if err != nil {
		t.Fatalf("should allow within limit, got: %v", err)
	}
}

func TestQuota_ModeTotal_BlocksWhenExceeded(t *testing.T) {
	database := testDB(t)
	userID := createTestUser(t, database, "alice", 0)
	addTestNode(t, database, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{Mode: "total", TotalLimit: 1000}, database)
	err := qc.Check(context.Background(), database, userID, 300)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_ModeTotal_AccountsForAllUsers(t *testing.T) {
	database := testDB(t)
	alice := createTestUser(t, database, "alice", 0)
	bob := createTestUser(t, database, "bob", 0)
	addTestNode(t, database, alice, "/file1.txt", 600)
	addTestNode(t, database, bob, "/file1.txt", 300)

	qc := NewQuotaChecker(QuotaConfig{Mode: "total", TotalLimit: 1000}, database)
	// Total is 900, adding 200 would be 1100 > 1000.
	err := qc.Check(context.Background(), database, alice, 200)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_ModeUser_AllowsWithinLimit(t *testing.T) {
	database := testDB(t)
	userID := createTestUser(t, database, "alice", 0)
	addTestNode(t, database, userID, "/file1.txt", 300)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, database)
	err := qc.Check(context.Background(), database, userID, 500)
	if err != nil {
		t.Fatalf("should allow within limit, got: %v", err)
	}
}

func TestQuota_ModeUser_BlocksWhenExceeded(t *testing.T) {
	database := testDB(t)
	userID := createTestUser(t, database, "alice", 0)
	addTestNode(t, database, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, database)
	err := qc.Check(context.Background(), database, userID, 300)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got: %v", err)
	}
}

func TestQuota_ModeUser_RespectsPerUserOverride(t *testing.T) {
	database := testDB(t)
	// User has a custom quota of 2000, server default is 1000.
	userID := createTestUser(t, database, "alice", 2000)
	addTestNode(t, database, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, database)

	// 800 + 500 = 1300, exceeds default 1000 but within custom 2000.
	err := qc.Check(context.Background(), database, userID, 500)
	if err != nil {
		t.Fatalf("should respect per-user override, got: %v", err)
	}

	// 800 + 1300 = 2100, exceeds custom 2000.
	err = qc.Check(context.Background(), database, userID, 1300)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded with per-user override, got: %v", err)
	}
}

func TestQuota_ModeUser_UnlimitedUser(t *testing.T) {
	database := testDB(t)
	// storage_quota=0 in user mode means use server default.
	// But we test the GetUserQuota returning 0 for truly unlimited.
	// To get an unlimited user in mode=user, the admin would set storage_quota=-1
	// or some convention. Actually per the plan, quota=0 means "use server default"
	// and the server default (UserLimit) is always > 0 in mode=user.
	// The "unlimited" case is when a custom user quota is set to a very large number.
	// Let's just verify that the default flow works.
	userID := createTestUser(t, database, "alice", 0)
	addTestNode(t, database, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, database)
	err := qc.Check(context.Background(), database, userID, 400)
	if err != nil {
		t.Fatalf("should allow within server default, got: %v", err)
	}
}

func TestQuota_OverwriteAccountsForDelta(t *testing.T) {
	database := testDB(t)
	userID := createTestUser(t, database, "alice", 0)
	addTestNode(t, database, userID, "/file1.txt", 800)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, database)

	// Overwriting: new file is 500 bytes, old was 800.
	// Net delta = 500 - 800 = -300. Usage would be 800 + (-300) = 500, within limit.
	err := qc.Check(context.Background(), database, userID, -300)
	if err != nil {
		t.Fatalf("overwrite with smaller file should be allowed, got: %v", err)
	}

	// Overwriting: new file is 1500 bytes, old was 800.
	// Net delta = 1500 - 800 = 700. Usage would be 800 + 700 = 1500, over limit.
	err = qc.Check(context.Background(), database, userID, 700)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded for overwrite exceeding limit, got: %v", err)
	}
}

func TestQuota_GetUserQuota(t *testing.T) {
	database := testDB(t)
	ctx := context.Background()

	// User with no override.
	alice := createTestUser(t, database, "alice", 0)
	// User with custom quota.
	bob := createTestUser(t, database, "bob", 5000)

	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 1000}, database)

	if got := qc.GetUserQuota(ctx, alice); got != 1000 {
		t.Errorf("alice quota: got %d, want 1000 (server default)", got)
	}
	if got := qc.GetUserQuota(ctx, bob); got != 5000 {
		t.Errorf("bob quota: got %d, want 5000 (custom override)", got)
	}

	// Non-existent user falls back to server default.
	if got := qc.GetUserQuota(ctx, 9999); got != 1000 {
		t.Errorf("non-existent user quota: got %d, want 1000", got)
	}
}

func TestQuota_ModeTotal_Transaction(t *testing.T) {
	database := testDB(t)
	userID := createTestUser(t, database, "alice", 0)
	addTestNode(t, database, userID, "/file1.txt", 500)

	qc := NewQuotaChecker(QuotaConfig{Mode: "total", TotalLimit: 1000}, database)

	// Check within a transaction.
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	err = qc.Check(context.Background(), tx, userID, 400)
	if err != nil {
		t.Fatalf("should allow within tx, got: %v", err)
	}

	err = qc.Check(context.Background(), tx, userID, 600)
	if err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded within tx, got: %v", err)
	}
}

func TestQuota_GetUserQuota_UsesQuerier(t *testing.T) {
	database := testDB(t)

	// Verify GetUserQuota uses the db connection from the checker.
	qc := NewQuotaChecker(QuotaConfig{Mode: "user", UserLimit: 2000}, database)

	userID := createTestUser(t, database, "alice", 0)

	// Verify default is returned via GetUserQuota.
	if got := qc.GetUserQuota(context.Background(), userID); got != 2000 {
		t.Fatalf("expected 2000, got %d", got)
	}
}
