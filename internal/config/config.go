package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	Addr             string  // GOSILO_ADDR — listen address
	BaseURL          string  // GOSILO_BASE_URL — public URL of the server
	DatabaseDSN      string  // GOSILO_DB — metadata database DSN (e.g. sqlite:gosilo.db)
	BlobDSN          string  // GOSILO_BLOB — blob store DSN (e.g. sqlite:blobs.db, fs:/path)
	RegistrationMode string  // GOSILO_REGISTRATION — "open" or "closed"
	LogLevel         string  // GOSILO_LOG_LEVEL — "debug", "info", "warn", "error"
	RateLimitRate    float64 // GOSILO_RATE_LIMIT — requests/sec per IP (0 = disabled)
	RateLimitBurst   int     // GOSILO_RATE_BURST — max burst size
	QuotaMode        string  // GOSILO_QUOTA_MODE — "off", "total", "user"
	QuotaTotal       int64   // GOSILO_QUOTA_TOTAL — bytes (parsed from human-readable)
	QuotaUser        int64   // GOSILO_QUOTA_USER — bytes (parsed from human-readable)
	MaxUploadSize    int64   // GOSILO_MAX_UPLOAD — max request body size (parsed from human-readable)
	WebMode          string  // GOSILO_WEB_MODE — "full", "oauth", or "off"
}

// ParseDSN splits a DSN string into its scheme and path components.
// For example, "sqlite:gosilo.db" returns ("sqlite", "gosilo.db", nil).
func ParseDSN(dsn string) (scheme, path string, err error) {
	i := strings.Index(dsn, ":")
	if i < 1 {
		return "", "", fmt.Errorf("invalid DSN %q: missing scheme (expected scheme:path)", dsn)
	}
	return dsn[:i], dsn[i+1:], nil
}

// Load reads configuration from environment variables, applying defaults where appropriate.
func Load() *Config {
	quotaTotal, _ := ParseByteSize(envOrDefault("GOSILO_QUOTA_TOTAL", "50GB"))
	quotaUser, _ := ParseByteSize(os.Getenv("GOSILO_QUOTA_USER"))
	maxUpload, _ := ParseByteSize(envOrDefault("GOSILO_MAX_UPLOAD", "50MB"))

	return &Config{
		Addr:             envOrDefault("GOSILO_ADDR", ":8080"),
		BaseURL:          envOrDefault("GOSILO_BASE_URL", "http://localhost:8080"),
		DatabaseDSN:      envOrDefault("GOSILO_DB", "sqlite:gosilo.db"),
		BlobDSN:          envOrDefault("GOSILO_BLOB", "sqlite:gosilo-blobs.db"),
		RegistrationMode: envOrDefault("GOSILO_REGISTRATION", "closed"),
		LogLevel:         envOrDefault("GOSILO_LOG_LEVEL", "info"),
		RateLimitRate:    envOrDefaultFloat("GOSILO_RATE_LIMIT", 10),
		RateLimitBurst:   envOrDefaultInt("GOSILO_RATE_BURST", 20),
		QuotaMode:        envOrDefault("GOSILO_QUOTA_MODE", "total"),
		QuotaTotal:       quotaTotal,
		QuotaUser:        quotaUser,
		MaxUploadSize:    maxUpload,
		WebMode:          envOrDefault("GOSILO_WEB_MODE", "full"),
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
