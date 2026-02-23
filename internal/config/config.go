package config

import (
	"os"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	Addr             string // GOSILO_ADDR — listen address
	BaseURL          string // GOSILO_BASE_URL — public URL of the server
	DatabasePath     string // GOSILO_DB_PATH — SQLite database file path
	BlobBackend      string // GOSILO_BLOB_BACKEND — blob storage backend type
	BlobPath         string // GOSILO_BLOB_PATH — path for filesystem blob backend
	RegistrationMode string // GOSILO_REGISTRATION — "open", "invite", or "closed"
	LogLevel         string // GOSILO_LOG_LEVEL — "debug", "info", "warn", "error"
}

// Load reads configuration from environment variables, applying defaults where appropriate.
func Load() *Config {
	return &Config{
		Addr:             envOrDefault("GOSILO_ADDR", ":8080"),
		BaseURL:          envOrDefault("GOSILO_BASE_URL", "http://localhost:8080"),
		DatabasePath:     envOrDefault("GOSILO_DB_PATH", "gosilo.db"),
		BlobBackend:      envOrDefault("GOSILO_BLOB_BACKEND", "sqlite"),
		BlobPath:         os.Getenv("GOSILO_BLOB_PATH"),
		RegistrationMode: envOrDefault("GOSILO_REGISTRATION", "closed"),
		LogLevel:         envOrDefault("GOSILO_LOG_LEVEL", "info"),
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
