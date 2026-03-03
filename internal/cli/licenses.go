package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"gosilo/internal/licenses"

	"github.com/spf13/cobra"
)

var licenseModule string

var licensesCmd = &cobra.Command{
	Use:     "licenses",
	Short:   "Show third-party license information",
	Long:    "Display license information for all open source dependencies included in this binary.",
	GroupID: "diagnostics",
	Example: `  gosilo licenses
  gosilo licenses --json
  gosilo licenses --module github.com/spf13/cobra`,
	RunE: runLicenses,
}

func init() {
	licensesCmd.Flags().StringVar(&licenseModule, "module", "", "show full license text for a specific module")
	rootCmd.AddCommand(licensesCmd)
}

func runLicenses(cmd *cobra.Command, args []string) error {
	data, err := licenses.Load()
	if err != nil {
		return fmt.Errorf("loading license data: %w", err)
	}

	// --module: show full license text for one module.
	if licenseModule != "" {
		return showModuleLicense(data, licenseModule)
	}

	// --json: full JSON output.
	if jsonFlag {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	// Default: table output.
	return showLicenseTable(data)
}

func showModuleLicense(data *licenses.LicenseData, module string) error {
	// Check project first.
	if module == data.Project.Name {
		fmt.Printf("%s (%s)\nLicense: %s\n\n%s\n", data.Project.Name, data.Project.Version, data.Project.LicenseType, data.Project.LicenseText)
		return nil
	}
	for _, dep := range data.Dependencies {
		if dep.Module == module {
			fmt.Printf("%s (%s)\nLicense: %s\n", dep.Module, dep.Version, dep.LicenseType)
			if dep.LicenseText != "" {
				fmt.Printf("\n%s\n", dep.LicenseText)
			} else {
				fmt.Println("\nNo license text available.")
			}
			return nil
		}
	}
	return fmt.Errorf("module %q not found", module)
}

func showLicenseTable(data *licenses.LicenseData) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "MODULE\tVERSION\tLICENSE\n")
	fmt.Fprintf(w, "------\t-------\t-------\n")

	// Project license first.
	fmt.Fprintf(w, "%s\t%s\t%s\n", data.Project.Name, data.Project.Version, data.Project.LicenseType)

	// Direct dependencies.
	for _, dep := range data.Dependencies {
		if dep.Direct {
			fmt.Fprintf(w, "%s\t%s\t%s\n", dep.Module, dep.Version, dep.LicenseType)
		}
	}

	// Indirect dependencies.
	for _, dep := range data.Dependencies {
		if !dep.Direct {
			fmt.Fprintf(w, "%s\t%s\t%s\n", dep.Module, dep.Version, dep.LicenseType)
		}
	}

	w.Flush()

	// Summary.
	direct := 0
	indirect := 0
	for _, dep := range data.Dependencies {
		if dep.Direct {
			direct++
		} else {
			indirect++
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d dependencies (%d direct, %d indirect)\n", len(data.Dependencies), direct, indirect)
	fmt.Fprintf(os.Stderr, "Generated: %s\n", data.Generated)
	return nil
}
