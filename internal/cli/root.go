package cli

import (
	"os"

	"gosilo/internal/config"

	"github.com/spf13/cobra"
)

var dbFlag string

var rootCmd = &cobra.Command{
	Use:     "gosilo",
	Short:   "Gosilo — remoteStorage server",
	Version: config.Version,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&dbFlag, "db", "", "metadata database DSN (overrides GOSILO_DB)")

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
	if v := os.Getenv("GOSILO_DB"); v != "" {
		return v
	}
	return fallback
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
