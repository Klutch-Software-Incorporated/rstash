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
	initProfile  string
	initPassword string
	initUsername  string
)

var initCmd = &cobra.Command{
	Use:     "init",
	Short:   "Create database and first admin user",
	Long:    "Initialize a new gosilo instance by creating the database and prompting for an admin account.",
	GroupID: "server",
	Example: `  gosilo init
  gosilo init --profile personal
  gosilo init --profile team --username admin --password "s3cure-p4ss"`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().StringVar(&initProfile, "profile", "", "configuration profile: personal, team, hardened")
	initCmd.Flags().StringVar(&initUsername, "username", "", "admin username (non-interactive)")
	initCmd.Flags().StringVar(&initPassword, "password", "", "admin password (non-interactive)")
	rootCmd.AddCommand(initCmd)
}

// profileSettings returns the DB settings overrides for a given profile.
func profileSettings(name string) (map[string]string, error) {
	switch name {
	case "personal":
		return map[string]string{
			"registration_mode": "closed",
			"rate_limit_rate":   "0",
		}, nil
	case "team":
		return map[string]string{
			"registration_mode": "open",
			"rate_limit_rate":   "10",
			"rate_limit_burst":  "20",
		}, nil
	case "hardened":
		return map[string]string{
			"registration_mode": "closed",
			"rate_limit_rate":   "5",
			"rate_limit_burst":  "10",
		}, nil
	default:
		return nil, fmt.Errorf("unknown profile %q (choose: personal, team, hardened)", name)
	}
}

// profileEnvHints returns suggested env vars for a profile.
func profileEnvHints(name string) string {
	switch name {
	case "personal":
		return `Recommended environment variables for "personal" profile:
  GOSILO_ADDR=127.0.0.1:8080
  GOSILO_WEB_MODE=oauth`
	case "team":
		return `Recommended environment variables for "team" profile:
  GOSILO_ADDR=:8080
  GOSILO_WEB_MODE=full`
	case "hardened":
		return `Recommended environment variables for "hardened" profile:
  GOSILO_ADDR=:8080
  GOSILO_WEB_MODE=oauth`
	default:
		return ""
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	dsn := resolvedDBDSN("sqlite:gosilo.db")

	// Validate profile early.
	if initProfile != "" {
		if _, err := profileSettings(initProfile); err != nil {
			return err
		}
	}

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
		return &SystemError{fmt.Errorf("create database: %w", err)}
	}
	defer database.Close()

	fmt.Fprintf(os.Stderr, "Database created at %s\n\n", dsn)

	// Get admin credentials.
	username := initUsername
	if username == "" {
		username, err = prompt("Admin username")
		if err != nil {
			return err
		}
	}
	if username == "" {
		return fmt.Errorf("username cannot be empty")
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
	localAuth := auth.NewLocalService(database)
	_, err = localAuth.CreateUser(context.Background(), username, password, true)
	if err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Admin user %q created.\n", username)

	// Apply profile settings.
	if initProfile != "" {
		overrides, _ := profileSettings(initProfile)
		cfg := config.Load()
		svc := settings.New(database, cfg)
		ctx := context.Background()
		for k, v := range overrides {
			if err := svc.Set(ctx, k, v); err != nil {
				return fmt.Errorf("apply profile setting %s=%s: %w", k, v, err)
			}
		}
		fmt.Fprintf(os.Stderr, "\nProfile %q applied.\n", initProfile)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, profileEnvHints(initProfile))
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
