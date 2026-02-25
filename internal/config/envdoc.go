package config

import "strings"

// EnvVar describes one environment variable used for configuration.
type EnvVar struct {
	Name         string   // Environment variable name.
	Default      string   // Default value (empty string if none).
	Description  string   // Human-readable description.
	ValidValues  []string // Allowed values, if restricted to a set.
	RequiredWhen string   // Condition under which this variable is required (empty = optional).
}

// EnvVars returns metadata for every configuration environment variable,
// derived from the canonical SettingDefs registry.
func EnvVars() []EnvVar {
	var out []EnvVar
	for _, d := range SettingDefs() {
		if d.EnvVar == "" {
			continue
		}
		out = append(out, EnvVar{
			Name:         d.EnvVar,
			Default:      d.Default,
			Description:  d.Description,
			ValidValues:  d.ValidValues,
			RequiredWhen: d.RequiredWhen,
		})
	}
	return out
}

// GenerateEnvFile returns a commented .env template suitable for writing to a file.
// All values are commented out so defaults take effect.
func GenerateEnvFile() string {
	var b strings.Builder
	b.WriteString("# Gosilo configuration\n")
	b.WriteString("# See: gosilo help\n")

	for _, v := range EnvVars() {
		b.WriteString("\n")
		b.WriteString("# " + v.Description + "\n")
		if len(v.ValidValues) > 0 {
			b.WriteString("# Valid values: " + strings.Join(v.ValidValues, ", ") + "\n")
		}
		if v.RequiredWhen != "" {
			b.WriteString("# Required when " + v.RequiredWhen + "\n")
		}
		if v.Default != "" {
			b.WriteString("# " + v.Name + "=" + v.Default + "\n")
		} else {
			b.WriteString("# " + v.Name + "=\n")
		}
	}

	return b.String()
}
