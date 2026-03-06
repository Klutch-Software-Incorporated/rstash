package cli

import (
	"fmt"

	"rstash/internal/config"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Print a documented .env template",
	Long:  "Print a commented .env configuration template to stdout. Redirect to a file to create a config file.",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print(config.GenerateEnvFile())
		return nil
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
