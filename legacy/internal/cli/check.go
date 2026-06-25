package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"rstash/internal/blob"
	"rstash/internal/config"
	"rstash/internal/db"
	"rstash/internal/email"

	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate configuration and test connections",
	Long:  "Load configuration from environment variables, validate all settings, and test database and blob store connectivity.",
	RunE:  runCheck,
}

func init() {
	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	cfg := config.Load()

	passed := 0
	failed := 0

	check := func(name string, fn func() error) {
		if err := fn(); err != nil {
			fmt.Fprintf(os.Stderr, "  FAIL  %s: %v\n", name, err)
			failed++
		} else {
			fmt.Fprintf(os.Stderr, "  OK    %s\n", name)
			passed++
		}
	}

	fmt.Fprintln(os.Stderr, "Checking configuration...")
	fmt.Fprintln(os.Stderr)

	check("Config validation", func() error {
		return cfg.Validate()
	})

	check("Database connection", func() error {
		repo, err := db.OpenRepository(cfg.DatabaseDSN)
		if err != nil {
			return err
		}
		defer repo.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err = repo.UserCount(ctx)
		return err
	})

	check("Blob store connection", func() error {
		scheme, path, err := config.ParseDSN(cfg.BlobDSN)
		if err != nil {
			return err
		}
		store, err := blob.OpenStore(scheme, path, cfg.BlobDSN)
		if err != nil {
			return err
		}
		defer store.Close()
		return nil
	})

	if cfg.EmailDSN != "" {
		check("Email provider", func() error {
			_, err := email.Open(cfg.EmailDSN)
			return err
		})
	}

	if cfg.TLSMode == "manual" || (cfg.TLSMode == "" && cfg.TLSCert != "") {
		check("TLS certificate file", func() error {
			_, err := os.Stat(cfg.TLSCert)
			return err
		})
		check("TLS key file", func() error {
			_, err := os.Stat(cfg.TLSKey)
			return err
		})
	}

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "%d passed, %d failed\n", passed, failed)

	if failed > 0 {
		return fmt.Errorf("%d check(s) failed", failed)
	}
	return nil
}
