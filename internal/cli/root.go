package cli

import (
	"errors"
	"os"

	"gosilo/internal/config"

	"github.com/spf13/cobra"
)

// Exit codes.
const (
	ExitOK          = 0 // Success
	ExitError       = 1 // General error
	ExitUsageError  = 2 // Bad arguments or usage
	ExitSystemError = 3 // System error (DB, IO, etc.)
)

// SystemError wraps errors that should exit with ExitSystemError.
type SystemError struct{ Err error }

func (e *SystemError) Error() string { return e.Err.Error() }
func (e *SystemError) Unwrap() error { return e.Err }

var (
	dbFlag   string
	jsonFlag bool
)

var rootCmd = &cobra.Command{
	Use:   "gosilo",
	Short: "Gosilo — remoteStorage server",
	Long: `Gosilo is a remoteStorage server implementing draft-dejong-remotestorage-26.

It provides WebFinger discovery, OAuth 2.0 authorization (implicit + PKCE),
and the full storage API (GET/PUT/DELETE/HEAD for documents and folders).

Running gosilo without a subcommand starts the HTTP server.`,
	Version: config.Version,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbFlag, "db", "", "metadata database DSN (overrides GOSILO_DB)")
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "output data as JSON")

	// Command groups for organized help output.
	rootCmd.AddGroup(
		&cobra.Group{ID: "server", Title: "Server:"},
		&cobra.Group{ID: "users", Title: "Users:"},
		&cobra.Group{ID: "settings", Title: "Settings:"},
		&cobra.Group{ID: "audit", Title: "Audit:"},
		&cobra.Group{ID: "diagnostics", Title: "Diagnostics:"},
	)

	// Default to serve when run without a subcommand.
	rootCmd.RunE = serveCmd.RunE
	rootCmd.Flags().AddFlagSet(serveCmd.Flags())
}

// resolvedDBDSN returns the DSN from --db flag, falling back to GOSILO_DB env
// or the provided default.
func resolvedDBDSN(fallback string) string {
	if dbFlag != "" {
		return dbFlag
	}
	if v := os.Getenv(config.EnvDB); v != "" {
		return v
	}
	return fallback
}

// RootCommand returns the root cobra command for introspection.
func RootCommand() *cobra.Command {
	return rootCmd
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		var sysErr *SystemError
		if errors.As(err, &sysErr) {
			os.Exit(ExitSystemError)
		}
		os.Exit(ExitError)
	}
}
