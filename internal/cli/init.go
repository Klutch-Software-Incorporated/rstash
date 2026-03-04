package cli

import (
	"context"
	"fmt"
	"os"

	"gosilo/internal/auth"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/settings"

	"github.com/spf13/cobra"
)

var (
	initPassword string
	initUsername  string
)

var initCmd = &cobra.Command{
	Use:     "init",
	Short:   "Create database and first admin user",
	Long:    "Initialize a new gosilo instance by creating the database and prompting for an admin account.",
	GroupID: "server",
	Example: `  gosilo init
  gosilo init --username admin --password "s3cure-p4ss"`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initUsername, "username", "", "admin username (non-interactive)")
	initCmd.Flags().StringVar(&initPassword, "password", "", "admin password (non-interactive)")
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
	repo, err := db.OpenRepository(dsn)
	if err != nil {
		return &SystemError{fmt.Errorf("create database: %w", err)}
	}
	defer repo.Close()

	fmt.Fprintf(os.Stderr, "Database created at %s\n\n", dsn)

	interactive := initUsername == ""

	// Get admin credentials.
	usernameInput := initUsername
	if usernameInput == "" {
		usernameInput, err = prompt("Admin username")
		if err != nil {
			return err
		}
	}
	username, err := db.ValidateUsername(usernameInput)
	if err != nil {
		return err
	}

	password := initPassword
	if password == "" {
		password, err = promptPassword("Admin password")
		if err != nil {
			return err
		}
		confirm, err := promptPassword("Confirm password")
		if err != nil {
			return err
		}
		if password != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	// Create admin user.
	localAuth := auth.NewLocalService(repo)
	_, err = localAuth.CreateUser(context.Background(), username, password, true, true)
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Admin user %q created.\n", username)

	// Ask setup questions interactively; use defaults when non-interactive.
	openRegistration := false
	rateLimiting := true

	if interactive {
		fmt.Fprintln(os.Stderr)
		openRegistration, err = promptYesNo("Allow public registration? (y/N):", false)
		if err != nil {
			return err
		}
		rateLimiting, err = promptYesNo("Enable rate limiting? (Y/n):", true)
		if err != nil {
			return err
		}
	}

	// Apply settings.
	overrides := map[string]string{}
	if openRegistration {
		overrides["registration_mode"] = "open"
	} else {
		overrides["registration_mode"] = "closed"
	}
	if rateLimiting {
		overrides["rate_limit_rate"] = "10"
		overrides["rate_limit_burst"] = "20"
	} else {
		overrides["rate_limit_rate"] = "0"
	}

	cfg := config.Load()
	svc := settings.New(repo, cfg)
	ctx := context.Background()
	for k, v := range overrides {
		if err := svc.Set(ctx, k, v); err != nil {
			return fmt.Errorf("apply setting %s=%s: %w", k, v, err)
		}
	}

	fmt.Fprintln(os.Stderr, "\nRun 'gosilo serve' to start the server.")
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
