package cli

import (
	"context"
	"fmt"
	"os"

	"gosilo/internal/config"
	"gosilo/internal/db"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run health checks on the database and configuration",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	dsn := resolvedDBDSN("sqlite:gosilo.db")
	ok := true

	// Check 1: config validation.
	cfg := config.Load()
	if dbFlag != "" {
		cfg.DatabaseDSN = dbFlag
	}
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] Config validation: %v\n", err)
		ok = false
	} else {
		fmt.Fprintln(os.Stderr, "[OK]   Config validation")
	}

	// Check 2: database open.
	database, err := db.Open(dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] Open database: %v\n", err)
		return fmt.Errorf("cannot continue without database")
	}
	defer database.Close()
	fmt.Fprintln(os.Stderr, "[OK]   Open database")

	// Check 3: SQLite integrity check.
	var integrityResult string
	if err := database.QueryRow("PRAGMA integrity_check").Scan(&integrityResult); err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] Integrity check: %v\n", err)
		ok = false
	} else if integrityResult != "ok" {
		fmt.Fprintf(os.Stderr, "[FAIL] Integrity check: %s\n", integrityResult)
		ok = false
	} else {
		fmt.Fprintln(os.Stderr, "[OK]   Integrity check")
	}

	// Check 4: user count.
	count, err := db.UserCount(context.Background(), database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] User count: %v\n", err)
		ok = false
	} else if count == 0 {
		fmt.Fprintln(os.Stderr, "[WARN] No users found — run 'gosilo init' or 'gosilo user add'")
	} else {
		fmt.Fprintf(os.Stderr, "[OK]   Users: %d\n", count)
	}

	// Check 5: storage stats.
	totalUsed, err := db.GetTotalStorageUsed(context.Background(), database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Storage stats: %v\n", err)
	} else {
		fmt.Fprintf(os.Stderr, "[OK]   Total storage used: %s\n", formatBytes(totalUsed))
	}

	if !ok {
		return fmt.Errorf("some checks failed")
	}
	return nil
}
