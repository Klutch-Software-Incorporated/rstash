package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"gosilo/internal/blob"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/settings"

	"github.com/spf13/cobra"
)

type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "ok", "warn", "fail"
	Detail string `json:"detail,omitempty"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run health checks on the database and configuration",
	Long: `Run diagnostic checks on the database, configuration, and blob store.

Checks include: config validation, database integrity, WAL mode, foreign keys,
schema completeness, user count, storage stats, expired sessions/tokens,
settings consistency, and blob store connectivity.`,
	GroupID: "diagnostics",
	Example: `  gosilo doctor
  gosilo doctor --json`,
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	dsn := resolvedDBDSN("sqlite:gosilo.db")
	hasFailure := false
	hasWarning := false
	var checks []checkResult

	add := func(name, status, detail string) {
		if status == "fail" {
			hasFailure = true
		}
		if status == "warn" {
			hasWarning = true
		}
		checks = append(checks, checkResult{name, status, detail})
		if !jsonFlag {
			tag := "[OK]  "
			if status == "fail" {
				tag = "[FAIL]"
			} else if status == "warn" {
				tag = "[WARN]"
			}
			if detail != "" {
				fmt.Fprintf(os.Stderr, "%s %s: %s\n", tag, name, detail)
			} else {
				fmt.Fprintf(os.Stderr, "%s %s\n", tag, name)
			}
		}
	}

	// Check 1: config validation.
	cfg := config.Load()
	if dbFlag != "" {
		cfg.DatabaseDSN = dbFlag
	}
	if err := cfg.Validate(); err != nil {
		add("Config validation", "fail", err.Error())
	} else {
		add("Config validation", "ok", "")
	}

	// Check 2: database open.
	database, err := db.Open(dsn)
	if err != nil {
		add("Open database", "fail", err.Error())
		if jsonFlag {
			return json.NewEncoder(os.Stdout).Encode(checks)
		}
		return &SystemError{fmt.Errorf("cannot continue without database")}
	}
	defer database.Close()
	add("Open database", "ok", "")

	ctx := context.Background()

	// Check 3: SQLite integrity check.
	var integrityResult string
	if err := database.QueryRow("PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		add("Integrity check", "fail", err.Error())
	} else if integrityResult != "ok" {
		add("Integrity check", "fail", integrityResult)
	} else {
		add("Integrity check", "ok", "")
	}

	// Check 4: WAL mode verification.
	var journalMode string
	if err := database.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		add("Journal mode", "fail", err.Error())
	} else if journalMode != "wal" {
		add("Journal mode", "warn", fmt.Sprintf("mode is %q (expected wal)", journalMode))
	} else {
		add("Journal mode", "ok", "WAL")
	}

	// Check 5: foreign keys enabled.
	var fkEnabled int
	if err := database.QueryRow("PRAGMA foreign_keys").Scan(&fkEnabled); err != nil {
		add("Foreign keys", "fail", err.Error())
	} else if fkEnabled != 1 {
		add("Foreign keys", "fail", "disabled")
	} else {
		add("Foreign keys", "ok", "enabled")
	}

	// Check 6: expected tables exist.
	expectedTables := []string{
		"users", "oauth_clients", "oauth_tokens", "nodes",
		"sessions", "audit_log", "authorization_codes", "settings",
	}
	rows, err := database.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		add("Schema", "fail", err.Error())
	} else {
		existing := map[string]bool{}
		for rows.Next() {
			var name string
			rows.Scan(&name)
			existing[name] = true
		}
		rows.Close()
		var missing []string
		for _, t := range expectedTables {
			if !existing[t] {
				missing = append(missing, t)
			}
		}
		if len(missing) > 0 {
			add("Schema", "fail", fmt.Sprintf("missing tables: %v", missing))
		} else {
			add("Schema", "ok", fmt.Sprintf("all %d tables present", len(expectedTables)))
		}
	}

	// Check 7: user count.
	count, err := db.UserCount(ctx, database)
	if err != nil {
		add("User count", "fail", err.Error())
	} else if count == 0 {
		add("User count", "warn", "no users found")
	} else {
		add("User count", "ok", fmt.Sprintf("%d", count))
	}

	// Check 8: storage stats.
	totalUsed, err := db.GetTotalStorageUsed(ctx, database)
	if err != nil {
		add("Storage stats", "warn", err.Error())
	} else {
		add("Storage stats", "ok", formatBytes(totalUsed))
	}

	// Check 9: expired sessions and tokens pending cleanup.
	var expiredSessions int64
	if err := database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sessions WHERE expires_at < datetime('now')").Scan(&expiredSessions); err != nil {
		add("Expired sessions", "warn", err.Error())
	} else if expiredSessions > 0 {
		add("Expired sessions", "warn", fmt.Sprintf("%d pending cleanup", expiredSessions))
	} else {
		add("Expired sessions", "ok", "none")
	}

	var expiredTokens int64
	if err := database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM oauth_tokens WHERE expires_at IS NOT NULL AND expires_at < datetime('now')").Scan(&expiredTokens); err != nil {
		add("Expired tokens", "warn", err.Error())
	} else if expiredTokens > 0 {
		add("Expired tokens", "warn", fmt.Sprintf("%d pending cleanup", expiredTokens))
	} else {
		add("Expired tokens", "ok", "none")
	}

	// Check 10: settings consistency.
	svc := settings.New(database, cfg)
	snap := svc.Load()
	if snap.QuotaMode == "user" && snap.QuotaUser == 0 {
		add("Settings consistency", "warn", "quota_mode=user but quota_user is not set")
	} else if snap.QuotaMode == "total" && snap.QuotaTotal == 0 {
		add("Settings consistency", "warn", "quota_mode=total but quota_total is not set")
	} else {
		add("Settings consistency", "ok", "")
	}

	// Check 11: blob store health.
	blobScheme, blobPath, blobErr := config.ParseDSN(cfg.BlobDSN)
	if blobErr != nil {
		add("Blob store", "fail", blobErr.Error())
	} else {
		var blobs blob.Store
		switch blobScheme {
		case "fs":
			blobs, err = blob.NewFSStore(blobPath)
		case "sqlite":
			blobs, err = blob.NewSQLiteStore(blobPath)
		default:
			err = fmt.Errorf("unknown blob backend: %q", blobScheme)
		}
		if err != nil {
			add("Blob store", "fail", err.Error())
		} else {
			blobs.Close()
			add("Blob store", "ok", blobScheme)
		}
	}

	if jsonFlag {
		return json.NewEncoder(os.Stdout).Encode(checks)
	}

	fmt.Fprintln(os.Stderr)
	if hasFailure {
		return fmt.Errorf("some checks failed")
	}
	if hasWarning {
		fmt.Fprintln(os.Stderr, "All checks passed with warnings.")
	} else {
		fmt.Fprintln(os.Stderr, "All checks passed.")
	}
	return nil
}
