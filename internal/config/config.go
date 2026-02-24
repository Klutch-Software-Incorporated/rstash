package config

import (
	"os"
	"strconv"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	Addr             string  // GOSILO_ADDR — listen address
	BaseURL          string  // GOSILO_BASE_URL — public URL of the server
	DatabasePath     string  // GOSILO_DB_PATH — SQLite database file path
	BlobBackend      string  // GOSILO_BLOB_BACKEND — blob storage backend type
	BlobPath         string  // GOSILO_BLOB_PATH — path for filesystem blob backend
	RegistrationMode string  // GOSILO_REGISTRATION — "open", "invite", or "closed"
	LogLevel         string  // GOSILO_LOG_LEVEL — "debug", "info", "warn", "error"
	RateLimitRate    float64 // GOSILO_RATE_LIMIT — requests/sec per IP (0 = disabled)
	RateLimitBurst   int     // GOSILO_RATE_BURST — max burst size
	QuotaMode        string  // GOSILO_QUOTA_MODE — "off", "total", "user"
	QuotaTotal       int64   // GOSILO_QUOTA_TOTAL — bytes (parsed from human-readable)
	QuotaUser        int64   // GOSILO_QUOTA_USER — bytes (parsed from human-readable)
}

// Load reads configuration from environment variables, applying defaults where appropriate.
func Load() *Config {
	quotaTotal, _ := ParseByteSize(envOrDefault("GOSILO_QUOTA_TOTAL", "50GB"))
	quotaUser, _ := ParseByteSize(os.Getenv("GOSILO_QUOTA_USER"))

	return &Config{
		Addr:             envOrDefault("GOSILO_ADDR", ":8080"),
		BaseURL:          envOrDefault("GOSILO_BASE_URL", "http://localhost:8080"),
		DatabasePath:     envOrDefault("GOSILO_DB_PATH", "gosilo.db"),
		BlobBackend:      envOrDefault("GOSILO_BLOB_BACKEND", "sqlite"),
		BlobPath:         os.Getenv("GOSILO_BLOB_PATH"),
		RegistrationMode: envOrDefault("GOSILO_REGISTRATION", "closed"),
		LogLevel:         envOrDefault("GOSILO_LOG_LEVEL", "info"),
		RateLimitRate:    envOrDefaultFloat("GOSILO_RATE_LIMIT", 10),
		RateLimitBurst:   envOrDefaultInt("GOSILO_RATE_BURST", 20),
		QuotaMode:        envOrDefault("GOSILO_QUOTA_MODE", "total"),
		QuotaTotal:       quotaTotal,
		QuotaUser:        quotaUser,
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrDefaultFloat(key string, defaultVal float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func envOrDefaultInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultVal
}
