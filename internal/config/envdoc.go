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

// EnvVars returns metadata for every configuration environment variable.
func EnvVars() []EnvVar {
	return []EnvVar{
		{
			Name:        "GOSILO_ADDR",
			Default:     ":8080",
			Description: "Listen address (host:port). Host may be empty to listen on all interfaces.",
		},
		{
			Name:        "GOSILO_BASE_URL",
			Default:     "http://localhost:8080",
			Description: "Public URL of the server. Used for WebFinger and OAuth redirects.",
		},
		{
			Name:        "GOSILO_DB",
			Default:     "sqlite:gosilo.db",
			Description: "Metadata database DSN. Only SQLite is supported (sqlite:path or sqlite::memory:).",
		},
		{
			Name:        "GOSILO_BLOB",
			Default:     "sqlite:gosilo-blobs.db",
			Description: "Blob store DSN. Supported schemes: sqlite:path, fs:/path/to/dir.",
		},
		{
			Name:        "GOSILO_REGISTRATION",
			Default:     "closed",
			Description: "User registration mode.",
			ValidValues: []string{"open", "closed"},
		},
		{
			Name:        "GOSILO_LOG_LEVEL",
			Default:     "info",
			Description: "Log verbosity level.",
			ValidValues: []string{"debug", "info", "warn", "error"},
		},
		{
			Name:        "GOSILO_RATE_LIMIT",
			Default:     "10",
			Description: "Per-IP rate limit in requests/sec. Set to 0 to disable.",
		},
		{
			Name:        "GOSILO_RATE_BURST",
			Default:     "20",
			Description: "Max burst size for per-IP rate limiting.",
		},
		{
			Name:        "GOSILO_QUOTA_MODE",
			Default:     "off",
			Description: "Storage quota enforcement mode.",
			ValidValues: []string{"off", "total", "user"},
		},
		{
			Name:         "GOSILO_QUOTA_TOTAL",
			Description:  "Global storage limit across all users (e.g. 10GB, 500MB).",
			RequiredWhen: "GOSILO_QUOTA_MODE=total",
		},
		{
			Name:         "GOSILO_QUOTA_USER",
			Description:  "Default per-user storage quota (e.g. 500MB, 1GB). Admin can override per user.",
			RequiredWhen: "GOSILO_QUOTA_MODE=user",
		},
		{
			Name:        "GOSILO_MAX_UPLOAD",
			Default:     "50MB",
			Description: "Maximum upload size per request (e.g. 50MB, 1GB).",
		},
		{
			Name:        "GOSILO_WEB_MODE",
			Default:     "full",
			Description: "Web UI mode. full=all routes, oauth=login+OAuth only, off=API only.",
			ValidValues: []string{"full", "oauth", "off"},
		},
	}
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
