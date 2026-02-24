package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"gosilo/internal/db"

	"github.com/spf13/cobra"
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "View audit log",
}

var auditTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Show recent audit log entries",
	RunE:  runAuditTail,
}

var auditExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export audit log as JSON lines",
	RunE:  runAuditExport,
}

var (
	auditTailN      int
	auditExportFmt  string
)

func init() {
	auditTailCmd.Flags().IntVarP(&auditTailN, "count", "n", 25, "number of entries to show")
	auditExportCmd.Flags().StringVar(&auditExportFmt, "format", "jsonl", "export format (jsonl)")

	auditCmd.AddCommand(auditTailCmd, auditExportCmd)
	rootCmd.AddCommand(auditCmd)
}

func runAuditTail(cmd *cobra.Command, args []string) error {
	dsn := resolvedDBDSN("sqlite:gosilo.db")
	database, err := db.Open(dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	entries, err := db.ListAuditEntries(context.Background(), database, auditTailN, 0)
	if err != nil {
		return fmt.Errorf("list audit entries: %w", err)
	}

	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No audit entries.")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Time\tActor\tAction\tTarget\tDetails")
	fmt.Fprintln(tw, "----\t-----\t------\t------\t-------")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s:%s\t%s\n",
			e.CreatedAt, e.ActorUsername, e.Action, e.TargetType, e.TargetID, e.Details)
	}
	return tw.Flush()
}

func runAuditExport(cmd *cobra.Command, args []string) error {
	dsn := resolvedDBDSN("sqlite:gosilo.db")
	database, err := db.Open(dsn)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	entries, err := db.ListAuditEntries(context.Background(), database, -1, 0)
	if err != nil {
		return fmt.Errorf("list audit entries: %w", err)
	}

	enc := json.NewEncoder(os.Stdout)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return fmt.Errorf("encode: %w", err)
		}
	}
	return nil
}
