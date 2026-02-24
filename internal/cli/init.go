package cli

import (
	"context"
	"fmt"
	"os"

	"gosilo/internal/auth"
	"gosilo/internal/db"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create database and first admin user",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	dsn := resolvedDBDSN("sqlite:gosilo.db")

	// Check if the DB file already exists (for file-based paths).
	_, path, err := parseDBPath(dsn)
	if err != nil {
		return err
	}
	if path != ":memory:" {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("database already exists at %s — use 'gosilo serve' to start, or delete the file to reinitialize", path)
		}
	}

	// Open (creates the database + runs migrations).
	database, err := db.Open(dsn)
	if err != nil {
		return fmt.Errorf("create database: %w", err)
	}
	defer database.Close()

	fmt.Fprintf(os.Stderr, "Database created at %s\n\n", dsn)

	// Prompt for admin credentials.
	username, err := prompt("Admin username")
	if err != nil {
		return err
	}
	if username == "" {
		return fmt.Errorf("username cannot be empty")
	}

	password, err := promptPassword("Admin password")
	if err != nil {
		return err
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	confirm, err := promptPassword("Confirm password")
	if err != nil {
		return err
	}
	if password != confirm {
		return fmt.Errorf("passwords do not match")
	}

	// Create admin user.
	localAuth := auth.NewLocalService(database)
	_, err = localAuth.CreateUser(context.Background(), username, password, true)
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nAdmin user %q created. Run 'gosilo serve' to start the server.\n", username)
	return nil
}

// parseDBPath extracts the scheme and file path from a DSN.
func parseDBPath(dsn string) (scheme, path string, err error) {
	scheme, path, err = parseDBDSN(dsn)
	if err != nil {
		return "", "", err
	}
	if scheme != "sqlite" {
		return "", "", fmt.Errorf("only sqlite is supported — got scheme %q", scheme)
	}
	return scheme, path, nil
}

func parseDBDSN(dsn string) (string, string, error) {
	for i := 0; i < len(dsn); i++ {
		if dsn[i] == ':' {
			return dsn[:i], dsn[i+1:], nil
		}
	}
	// No scheme — treat as bare path.
	return "sqlite", dsn, nil
}
