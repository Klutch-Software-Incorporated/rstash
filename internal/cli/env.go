package cli

import (
	"fmt"

	"gosilo/internal/config"

	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Print a documented .env template",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(config.GenerateEnvFile())
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
