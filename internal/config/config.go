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
	TokenLifetime        string  // GOSILO_TOKEN_LIFETIME — OAuth token lifetime: "30d", "24h", "0" (no expiry)
	RefreshTokens        string  // "enabled" or "disabled"
	RefreshTokenLifetime string  // refresh token lifetime: "90d", "0" (no expiry)
	MetricsMode          string  // "public", "admin", or "off"
	JSONApi              string  // "off" or "admin"
	LogFile              string  // GOSILO_LOG_FILE — path to log file (empty = stderr only)
	TLSCert              string  // GOSILO_TLS_CERT — path to TLS certificate file
	TLSKey               string  // GOSILO_TLS_KEY — path to TLS private key file
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

// Load reads configuration from environment variables (for boot-critical
// settings) and hardcoded defaults (for runtime-editable settings that
// have no env var). Runtime-editable settings are managed via the CLI
// or admin UI and stored in the database.
func Load() *Config {
	quotaTotal, _ := ParseByteSize("50GB")
	maxUpload, _ := ParseByteSize("50MB")

	return &Config{
		// Boot-critical: read from env vars.
		Addr:        envOrDefault(EnvAddr, ":8080"),
		BaseURL:     envOrDefault(EnvBaseURL, "http://localhost:8080"),
		DatabaseDSN: envOrDefault(EnvDB, "sqlite:gosilo.db"),
		BlobDSN:     envOrDefault(EnvBlob, "sqlite:gosilo-blobs.db"),
		WebMode:     envOrDefault(EnvWebMode, "full"),
		LogLevel:    envOrDefault(EnvLogLevel, "info"),
		LogFile:     os.Getenv(EnvLogFile),
		TLSCert:     os.Getenv(EnvTLSCert),
		TLSKey:      os.Getenv(EnvTLSKey),

		// Runtime-editable: sane defaults, changed via CLI/admin UI.
		MetricsMode:      "public",
		JSONApi:          "off",
		RegistrationMode: "closed",
		RateLimitRate:    10,
		RateLimitBurst:   20,
		QuotaMode:        "total",
		QuotaTotal:       quotaTotal,
		QuotaUser:        0,
		MaxUploadSize:    maxUpload,
		TokenLifetime:        "30d",
		RefreshTokens:        "enabled",
		RefreshTokenLifetime: "90d",
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

// ValueMap returns all env-sourced setting values as a key→display-string map,
// keyed by the SettingDef.Key. This is used by the admin UI and CLI to display
// current values for env-only (non-runtime-editable) settings.
func (c *Config) ValueMap() map[string]string {
	return map[string]string{
		"addr":              c.Addr,
		"base_url":          c.BaseURL,
		"database_dsn":      c.DatabaseDSN,
		"blob_dsn":          c.BlobDSN,
		"metrics_mode":      c.MetricsMode,
		"json_api":          c.JSONApi,
		"log_file":          c.LogFile,
		"registration_mode": c.RegistrationMode,
		"log_level":         c.LogLevel,
		"rate_limit_rate":   fmt.Sprintf("%g", c.RateLimitRate),
		"rate_limit_burst":  fmt.Sprintf("%d", c.RateLimitBurst),
		"quota_mode":        c.QuotaMode,
		"quota_total":       FormatByteSize(c.QuotaTotal),
		"quota_user":        FormatByteSize(c.QuotaUser),
		"max_upload_size":   FormatByteSize(c.MaxUploadSize),
		"web_mode":          c.WebMode,
		"token_lifetime":         c.TokenLifetime,
		"refresh_tokens":         c.RefreshTokens,
		"refresh_token_lifetime": c.RefreshTokenLifetime,
		"tls_cert":               c.TLSCert,
		"tls_key":                c.TLSKey,
	}
}
