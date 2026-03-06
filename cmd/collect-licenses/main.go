// Command collect-licenses scans Go module dependencies and collects their
// license texts into a JSON file for embedding into the rstash binary.
//
// It uses only the standard library — no external dependencies required.
//
// Usage:
//
//	go run ./cmd/collect-licenses
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// Output types — must match internal/licenses.LicenseData.

type LicenseData struct {
	Generated    string              `json:"generated"`
	Project      ProjectLicense      `json:"project"`
	Dependencies []DependencyLicense `json:"dependencies"`
}

type ProjectLicense struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	LicenseType string `json:"license_type"`
	LicenseText string `json:"license_text"`
}

type DependencyLicense struct {
	Module      string `json:"module"`
	Version     string `json:"version"`
	LicenseType string `json:"license_type"`
	LicenseText string `json:"license_text"`
	Direct      bool   `json:"direct"`
}

// Module represents one entry from `go list -m -json all`.
type Module struct {
	Path     string `json:"Path"`
	Version  string `json:"Version"`
	Dir      string `json:"Dir"`
	Indirect bool   `json:"Indirect"`
	Main     bool   `json:"Main"`
}

// ManualOverrides is the structure of licenses-manual.json.
type ManualOverrides struct {
	Overrides map[string]struct {
		LicenseType string `json:"license_type"`
		LicenseText string `json:"license_text"`
	} `json:"overrides"`
	Additions []DependencyLicense `json:"additions"`
}

// License file names to search for, in priority order.
var licenseFileNames = []string{
	"LICENSE",
	"LICENSE.md",
	"LICENSE.txt",
	"LICENCE",
	"LICENCE.md",
	"LICENCE.txt",
	"COPYING",
	"COPYING.md",
	"COPYING.txt",
	"LICENSE-MIT",
	"LICENSE-APACHE",
	"LICENSE.MIT",
	"LICENSE.APACHE",
}

func main() {
	// 1. Run `go list -m -json all` to enumerate modules.
	modules, err := listModules()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error listing modules: %v\n", err)
		os.Exit(1)
	}

	// 2. Download modules missing Dir and build a directory lookup.
	dirMap := downloadMissingDirs(modules)

	// 3. Load manual overrides.
	manual := loadManualOverrides()

	// 4. Collect licenses for each dependency.
	var deps []DependencyLicense
	for _, mod := range modules {
		if mod.Main {
			continue
		}

		dep := DependencyLicense{
			Module:  mod.Path,
			Version: mod.Version,
			Direct:  !mod.Indirect,
		}

		// Check for manual override first.
		if ov, ok := manual.Overrides[mod.Path]; ok {
			dep.LicenseType = ov.LicenseType
			dep.LicenseText = ov.LicenseText
			deps = append(deps, dep)
			continue
		}

		// Resolve directory: prefer go list Dir, fall back to download Dir.
		dir := mod.Dir
		if dir == "" {
			dir = dirMap[mod.Path]
		}

		// Search for license file in module directory.
		if dir != "" {
			text, file := findLicenseFile(dir)
			if text != "" {
				dep.LicenseText = text
				dep.LicenseType = classifyLicense(text)
				_ = file
			} else {
				// Walk up parent directories for shared licenses
				// (e.g., golang.org/x/* modules share a root LICENSE).
				text, _ = findLicenseInParents(dir)
				if text != "" {
					dep.LicenseText = text
					dep.LicenseType = classifyLicense(text)
				}
			}
		}

		if dep.LicenseType == "" {
			dep.LicenseType = "Unknown"
		}

		deps = append(deps, dep)
	}

	// Add manual additions.
	deps = append(deps, manual.Additions...)

	// Sort: direct first, then by module path.
	sort.Slice(deps, func(i, j int) bool {
		if deps[i].Direct != deps[j].Direct {
			return deps[i].Direct
		}
		return deps[i].Module < deps[j].Module
	})

	// 5. Read project's own LICENSE.
	projectLicense := ""
	if data, err := os.ReadFile("LICENSE"); err == nil {
		projectLicense = string(data)
	}

	// 6. Build output.
	output := LicenseData{
		Generated: time.Now().UTC().Format(time.RFC3339),
		Project: ProjectLicense{
			Name:        "rstash",
			Version:     "dev",
			LicenseType: "MIT",
			LicenseText: projectLicense,
		},
		Dependencies: deps,
	}

	// 7. Write to internal/licenses/licenses.json.
	outDir := filepath.Join("internal", "licenses")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output directory: %v\n", err)
		os.Exit(1)
	}

	outPath := filepath.Join(outDir, "licenses.json")
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outPath, err)
		os.Exit(1)
	}

	fmt.Printf("Collected licenses for %d dependencies → %s\n", len(deps), outPath)
}

// listModules runs `go list -m -json all` and parses the output.
// The output is a stream of JSON objects (not a JSON array).
func listModules() ([]Module, error) {
	cmd := exec.Command("go", "list", "-m", "-json", "all")
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -m -json all: %w", err)
	}

	var modules []Module
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var mod Module
		if err := dec.Decode(&mod); err != nil {
			return nil, fmt.Errorf("decoding module JSON: %w", err)
		}
		modules = append(modules, mod)
	}
	return modules, nil
}

// DownloadInfo represents one entry from `go mod download -json`.
type DownloadInfo struct {
	Path    string `json:"Path"`
	Version string `json:"Version"`
	Dir     string `json:"Dir"`
	Error   string `json:"Error,omitempty"`
}

// downloadMissingDirs runs `go mod download -json` for modules that lack a Dir
// from `go list`. Returns a map of module path → directory.
func downloadMissingDirs(modules []Module) map[string]string {
	var missing []string
	for _, mod := range modules {
		if mod.Main || mod.Dir != "" || mod.Version == "" {
			continue
		}
		missing = append(missing, mod.Path+"@"+mod.Version)
	}
	if len(missing) == 0 {
		return nil
	}

	fmt.Printf("Downloading %d modules missing from cache...\n", len(missing))
	args := append([]string{"mod", "download", "-json"}, missing...)
	cmd := exec.Command("go", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: go mod download failed: %v\n", err)
		return nil
	}

	dirMap := make(map[string]string)
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var info DownloadInfo
		if err := dec.Decode(&info); err != nil {
			continue
		}
		if info.Dir != "" {
			dirMap[info.Path] = info.Dir
		}
	}
	return dirMap
}

// findLicenseFile searches a directory for a license file.
func findLicenseFile(dir string) (text string, filename string) {
	for _, name := range licenseFileNames {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data), name
		}
		// Also try lowercase.
		path = filepath.Join(dir, strings.ToLower(name))
		data, err = os.ReadFile(path)
		if err == nil {
			return string(data), strings.ToLower(name)
		}
	}
	return "", ""
}

// findLicenseInParents walks up from dir looking for a license file in parent
// directories. This handles cases like golang.org/x/text where the LICENSE
// is in the golang.org/x/ root. Stops after 3 levels to avoid going too far.
func findLicenseInParents(dir string) (string, string) {
	current := dir
	for i := 0; i < 3; i++ {
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		text, name := findLicenseFile(parent)
		if text != "" {
			return text, name
		}
		current = parent
	}
	return "", ""
}

// loadManualOverrides reads licenses-manual.json if it exists.
func loadManualOverrides() ManualOverrides {
	var m ManualOverrides
	data, err := os.ReadFile("licenses-manual.json")
	if err != nil {
		return ManualOverrides{
			Overrides: make(map[string]struct {
				LicenseType string `json:"license_type"`
				LicenseText string `json:"license_text"`
			}),
		}
	}
	if err := json.Unmarshal(data, &m); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not parse licenses-manual.json: %v\n", err)
		return ManualOverrides{
			Overrides: make(map[string]struct {
				LicenseType string `json:"license_type"`
				LicenseText string `json:"license_text"`
			}),
		}
	}
	if m.Overrides == nil {
		m.Overrides = make(map[string]struct {
			LicenseType string `json:"license_type"`
			LicenseText string `json:"license_text"`
		})
	}
	return m
}

// classifyLicense attempts to determine the license type from its text.
func classifyLicense(text string) string {
	// Normalize whitespace for matching.
	normalized := normalizeWhitespace(strings.ToLower(text))

	// Check for specific license identifiers.
	switch {
	case strings.Contains(normalized, "apache license") && strings.Contains(normalized, "version 2.0"):
		return "Apache-2.0"
	case strings.Contains(normalized, "mozilla public license") && strings.Contains(normalized, "version 2.0"):
		return "MPL-2.0"
	case strings.Contains(normalized, "gnu lesser general public license"):
		return "LGPL"
	case strings.Contains(normalized, "gnu general public license"):
		return "GPL"
	case strings.Contains(normalized, "isc license") || (strings.Contains(normalized, "permission to use, copy, modify, and/or distribute") && strings.Contains(normalized, "isc")):
		return "ISC"
	case isMIT(normalized):
		return "MIT"
	case isBSD3(normalized):
		return "BSD-3-Clause"
	case isBSD2(normalized):
		return "BSD-2-Clause"
	case strings.Contains(normalized, "unlicense"):
		return "Unlicense"
	case strings.Contains(normalized, "creative commons"):
		return "CC"
	case strings.Contains(normalized, "boost software license"):
		return "BSL-1.0"
	default:
		return "Unknown"
	}
}

func isMIT(text string) bool {
	return strings.Contains(text, "mit license") ||
		(strings.Contains(text, "permission is hereby granted, free of charge") &&
			strings.Contains(text, "the above copyright notice"))
}

func isBSD3(text string) bool {
	return (strings.Contains(text, "redistribution and use in source and binary forms") &&
		strings.Contains(text, "neither the name"))
}

func isBSD2(text string) bool {
	return (strings.Contains(text, "redistribution and use in source and binary forms") &&
		!strings.Contains(text, "neither the name") &&
		strings.Contains(text, "provided that the following conditions"))
}

func normalizeWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !prevSpace {
				b.WriteByte(' ')
			}
			prevSpace = true
		} else {
			b.WriteRune(r)
			prevSpace = false
		}
	}
	return b.String()
}
