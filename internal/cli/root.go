package cli

import (
	"errors"
	"os"

	"rstash/internal/config"

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

var rootCmd = &cobra.Command{
	Use:   "rstash",
	Short: "rstash — remoteStorage server",
	Long: `rstash is a remoteStorage server implementing draft-dejong-remotestorage-26.

It provides WebFinger discovery, OAuth 2.0 authorization (implicit + PKCE),
and the full storage API (GET/PUT/DELETE/HEAD for documents and folders).

Running rstash starts the HTTP server.`,
	Version: config.Version,
}

func init() {
	// Default to serve when run without a subcommand.
	rootCmd.RunE = serveCmd.RunE
	rootCmd.Flags().AddFlagSet(serveCmd.Flags())

	// Disable the auto-generated completion subcommand.
	rootCmd.CompletionOptions.DisableDefaultCmd = true
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
