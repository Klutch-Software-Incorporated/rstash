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
	Use:     "audit",
	Short:   "View audit log",
	Long:    "Browse and export the audit log of all administrative and storage events.",
	GroupID: "audit",
}

var auditTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Show recent audit log entries",
	Long: `Show the most recent audit log entries in a formatted table. Each entry includes
the timestamp, actor (user or "system"), action, target, and details. Use -n to
control how many entries are shown (default 25). Use --json for machine-readable
JSON array output.`,
	Example: `  gosilo audit tail
  gosilo audit tail -n 50
  gosilo audit tail --json`,
	RunE: runAuditTail,
}

var auditExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export audit log as JSON lines",
	Long: `Export the entire audit log as JSON Lines (one JSON object per line). Each line
contains all fields for a single audit entry. This format is ideal for piping
to files, log aggregators, or processing with tools like jq.`,
	Example: `  gosilo audit export > audit.jsonl`,
	RunE:    runAuditExport,
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
	repo, err := db.OpenRepository(dsn)
	if err != nil {
		return &SystemError{fmt.Errorf("open database: %w", err)}
	}
	defer repo.Close()

	entries, err := repo.ListAuditEntries(context.Background(), auditTailN, 0)
	if err != nil {
		return fmt.Errorf("list audit entries: %w", err)
	}

	if len(entries) == 0 {
		if jsonFlag {
			fmt.Println("[]")
			return nil
		}
		fmt.Fprintln(os.Stderr, "No audit entries.")
		return nil
	}

	if jsonFlag {
		type auditJSON struct {
			Time       string `json:"time"`
			Actor      string `json:"actor"`
			Action     string `json:"action"`
			TargetType string `json:"target_type"`
			TargetID   string `json:"target_id"`
			Details    string `json:"details,omitempty"`
		}
		out := make([]auditJSON, len(entries))
		for i, e := range entries {
			out[i] = auditJSON{e.CreatedAt.Format("2006-01-02 15:04:05"), e.ActorUsername, e.Action, e.TargetType, e.TargetID, e.Details}
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Time\tActor\tAction\tTarget\tDetails")
	fmt.Fprintln(tw, "----\t-----\t------\t------\t-------")
	for _, e := range entries {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s:%s\t%s\n",
			e.CreatedAt.Format("2006-01-02 15:04:05"), e.ActorUsername, e.Action, e.TargetType, e.TargetID, e.Details)
	}
	return tw.Flush()
}

func runAuditExport(cmd *cobra.Command, args []string) error {
	dsn := resolvedDBDSN("sqlite:gosilo.db")
	repo, err := db.OpenRepository(dsn)
	if err != nil {
		return &SystemError{fmt.Errorf("open database: %w", err)}
	}
	defer repo.Close()

	entries, err := repo.ListAuditEntries(context.Background(), -1, 0)
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
