package db

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    is_admin INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS oauth_clients (
    id TEXT PRIMARY KEY,
    redirect_uri TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS oauth_tokens (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    client_id TEXT NOT NULL REFERENCES oauth_clients(id),
    scopes TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT
);

CREATE TABLE IF NOT EXISTS nodes (
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
);

CREATE TABLE IF NOT EXISTS sessions (
    token TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY,
    actor_id INTEGER NOT NULL REFERENCES users(id),
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    details TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);

CREATE TABLE IF NOT EXISTS authorization_codes (
    code TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id),
    client_id TEXT NOT NULL REFERENCES oauth_clients(id),
    redirect_uri TEXT NOT NULL,
    scopes TEXT NOT NULL,
    code_challenge TEXT NOT NULL,
    code_challenge_method TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT NOT NULL DEFAULT (datetime('now', '+10 minutes')),
    used INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
`

// Open opens a SQLite database at the given DSN, enables WAL mode and
// foreign keys, and runs schema migrations.
// The dsn may be a bare path or a "sqlite:path" DSN; the "sqlite:" prefix
// is stripped if present.
func Open(dsn string) (*sql.DB, error) {
	path := dsn
	if after, ok := strings.CutPrefix(dsn, "sqlite:"); ok {
		path = after
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL mode: %w", err)
	}

	// Enable foreign key enforcement.
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Run schema migrations.
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	// Run column migrations (safe to re-run).
	if err := runMigrations(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("run column migrations: %w", err)
	}

	// Ensure the _system sentinel user exists (for CLI/system audit entries).
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO users (id, username, password_hash, is_admin, disabled)
		 VALUES (0, '_system', '', 0, 1)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create _system user: %w", err)
	}

	return db, nil
}

// runMigrations applies ALTER TABLE migrations, ignoring "duplicate column" errors.
func runMigrations(database *sql.DB) error {
	migrations := []string{
		"ALTER TABLE users ADD COLUMN storage_quota INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE users ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0",
	}
	for _, m := range migrations {
		if _, err := database.Exec(m); err != nil {
			// Ignore "duplicate column" errors (column already exists).
			if !isDuplicateColumnError(err) {
				return err
			}
		}
	}
	return nil
}

// isDuplicateColumnError checks if the error indicates the column already exists.
func isDuplicateColumnError(err error) bool {
	return err != nil && (contains(err.Error(), "duplicate column") || contains(err.Error(), "already exists"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
