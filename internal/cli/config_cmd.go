package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/settings"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Manage runtime settings",
	Long:    "View and modify runtime settings. DB overrides take precedence over environment variables.",
	GroupID: "settings",
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all settings with source",
	Long: `List all runtime-editable settings with their current values and source.

The "source" column indicates where each value comes from:
  db   — value is set as a database override (takes precedence)
  env  — value comes from an environment variable or default

Use --json to get machine-readable output as a JSON array of objects
with "key", "value", and "source" fields.`,
	Example: `  gosilo config list
  gosilo config list --json`,
	RunE: runConfigList,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get the resolved value for a setting",
	Long: `Get the current resolved value for a single runtime setting.

Resolution order: if a database override exists, it is returned; otherwise
the value from the environment variable (or its default) is used. Use --json
to get the result as a JSON object with "key" and "value" fields.`,
	Example: `  gosilo config get registration_mode
  gosilo config get quota_user --json`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a runtime setting override",
	Long: `Set a database override for a runtime setting.

The value is validated against the setting's type and allowed values before
being stored. The change takes effect immediately for the running server
(no restart required). The override persists across server restarts in the
metadata database.`,
	Example: `  gosilo config set registration_mode open
  gosilo config set quota_user 100MB
  gosilo config set token_lifetime 7d`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configResetCmd = &cobra.Command{
	Use:   "reset <key>",
	Short: "Remove a setting override (revert to env default)",
	Long: `Remove a database override for a setting, reverting it to the value from
the environment variable (or its default). If no override exists, this
is a no-op. The change takes effect immediately for the running server.`,
	Example: `  gosilo config reset registration_mode`,
	Args:    cobra.ExactArgs(1),
	RunE:    runConfigReset,
}

func init() {
	configCmd.AddCommand(configListCmd, configGetCmd, configSetCmd, configResetCmd)
	rootCmd.AddCommand(configCmd)
}

func openSettings() (*settings.Settings, *sql.DB, func(), error) {
	dsn := resolvedDBDSN("sqlite:gosilo.db")
	database, err := db.Open(dsn)
	if err != nil {
		return nil, nil, nil, &SystemError{fmt.Errorf("open database: %w", err)}
	}
	cfg := config.Load()
	if dbFlag != "" {
		cfg.DatabaseDSN = dbFlag
	}
	s := settings.New(database, cfg)
	return s, database, func() { database.Close() }, nil
}

func runConfigList(cmd *cobra.Command, args []string) error {
	s, _, cleanup, err := openSettings()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	snap := s.Load()
	overrides, err := s.Overrides(ctx)
	if err != nil {
		return fmt.Errorf("load overrides: %w", err)
	}

	valueMap := snap.ValueMap()

	type row struct {
		key, value, source string
	}
	var rows []row
	for _, def := range config.RuntimeSettingDefs() {
		source := "env"
		if _, ok := overrides[def.Key]; ok {
			source = "db"
		}
		rows = append(rows, row{def.Key, valueMap[def.Key], source})
	}

	if jsonFlag {
		type settingJSON struct {
			Key    string `json:"key"`
			Value  string `json:"value"`
			Source string `json:"source"`
		}
		out := make([]settingJSON, len(rows))
		for i, r := range rows {
			out[i] = settingJSON{r.key, r.value, r.source}
		}
		return json.NewEncoder(os.Stdout).Encode(out)
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Key\tValue\tSource")
	fmt.Fprintln(tw, "---\t-----\t------")
	for _, r := range rows {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", r.key, r.value, r.source)
	}
	return tw.Flush()
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	def := config.SettingDefByKey(key)
	if def == nil || !def.RuntimeEditable {
		return fmt.Errorf("unknown runtime setting: %q", key)
	}

	s, _, cleanup, err := openSettings()
	if err != nil {
		return err
	}
	defer cleanup()

	snap := s.Load()
	val := snap.ValueMap()[key]

	if jsonFlag {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{"key": key, "value": val})
	}

	fmt.Println(val)
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key, value := args[0], args[1]

	s, database, cleanup, err := openSettings()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	if err := s.Set(ctx, key, value); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}

	db.Audit(ctx, database, db.SystemActorID, "settings.updated", "setting", key, value)
	fmt.Fprintf(os.Stderr, "Set %s = %s\n", key, value)
	return nil
}

func runConfigReset(cmd *cobra.Command, args []string) error {
	key := args[0]

	s, database, cleanup, err := openSettings()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx := context.Background()
	if err := s.Delete(ctx, key); err != nil {
		return fmt.Errorf("reset %s: %w", key, err)
	}

	db.Audit(ctx, database, db.SystemActorID, "settings.reset", "setting", key, "reverted to default")
	fmt.Fprintf(os.Stderr, "Reset %s to default.\n", key)
	return nil
}
