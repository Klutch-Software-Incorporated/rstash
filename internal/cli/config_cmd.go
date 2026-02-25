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
	Example: `  gosilo config list
  gosilo config list --json`,
	RunE: runConfigList,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get the resolved value for a setting",
	Example: `  gosilo config get registration_mode
  gosilo config get quota_user --json`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a runtime setting override",
	Example: `  gosilo config set registration_mode open
  gosilo config set quota_user 100MB
  gosilo config set token_lifetime 7d`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configResetCmd = &cobra.Command{
	Use:   "reset <key>",
	Short: "Remove a setting override (revert to env default)",
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

	type row struct {
		key, value, source string
	}
	rows := []row{
		{"registration_mode", snap.RegistrationMode, ""},
		{"log_level", snap.LogLevel, ""},
		{"rate_limit_rate", fmt.Sprintf("%g", snap.RateLimitRate), ""},
		{"rate_limit_burst", fmt.Sprintf("%d", snap.RateLimitBurst), ""},
		{"quota_mode", snap.QuotaMode, ""},
		{"quota_total", config.FormatByteSize(snap.QuotaTotal), ""},
		{"quota_user", config.FormatByteSize(snap.QuotaUser), ""},
		{"max_upload_size", config.FormatByteSize(snap.MaxUploadSize), ""},
		{"token_lifetime", snap.TokenLifetime, ""},
	}
	for i := range rows {
		if _, ok := overrides[rows[i].key]; ok {
			rows[i].source = "db"
		} else {
			rows[i].source = "env"
		}
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

	s, _, cleanup, err := openSettings()
	if err != nil {
		return err
	}
	defer cleanup()

	snap := s.Load()

	var val string
	switch key {
	case "registration_mode":
		val = snap.RegistrationMode
	case "log_level":
		val = snap.LogLevel
	case "rate_limit_rate":
		val = fmt.Sprintf("%g", snap.RateLimitRate)
	case "rate_limit_burst":
		val = fmt.Sprintf("%d", snap.RateLimitBurst)
	case "quota_mode":
		val = snap.QuotaMode
	case "quota_total":
		val = config.FormatByteSize(snap.QuotaTotal)
	case "quota_user":
		val = config.FormatByteSize(snap.QuotaUser)
	case "max_upload_size":
		val = config.FormatByteSize(snap.MaxUploadSize)
	case "token_lifetime":
		val = snap.TokenLifetime
	default:
		return fmt.Errorf("unknown setting: %q", key)
	}

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
