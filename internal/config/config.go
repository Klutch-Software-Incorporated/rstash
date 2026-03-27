package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const defaultTOSContent = "Terms of Service\n\n1. Acceptance. By creating an account or using this service, you agree to these terms.\n2. Acceptable Use. You shall not use this service to store, transmit, or distribute content that is unlawful, harmful, threatening, abusive, defamatory, or otherwise objectionable.\n3. Prohibited Content. You shall not store content that: (a) violates any applicable law or regulation; (b) infringes any intellectual property right; (c) contains child sexual abuse material; or (d) contains malware or other harmful code.\n4. Termination. The operator may suspend or terminate your account at any time, with or without cause or notice.\n5. No Warranty. This service is provided \"as is\" without warranty of any kind, express or implied.\n6. Limitation of Liability. The operator shall not be liable for any indirect, incidental, special, consequential, or punitive damages.\n7. Modifications. These terms may be updated at any time. Continued use constitutes acceptance."

const defaultPrivacyContent = "Privacy Policy\n\n1. Data Collected. This service stores account credentials (username, hashed password) and files you upload. Server logs may record IP addresses, timestamps, and user agents.\n2. Use of Data. Your data is used solely to provide the service. The operator does not sell, share, or disclose your data to third parties except as required by law.\n3. Data Access. The server operator has technical access to the infrastructure. Your files are not routinely accessed by the operator.\n4. Data Retention. Your data is retained for the duration of your account. Upon deletion, your data is permanently removed.\n5. Security. Reasonable measures are taken to protect your data. No method of storage or transmission is completely secure.\n6. Contact. For questions, contact the server operator."

// Config holds all application configuration, loaded from environment variables.
type Config struct {
	Addr             string  // RSTASH_ADDR — listen address
	BaseURL          string  // RSTASH_BASE_URL — public URL of the server
	DatabaseDSN      string  // RSTASH_DB — metadata database DSN (e.g. sqlite:rstash.db)
	BlobDSN          string  // RSTASH_BLOB — blob store DSN (e.g. sqlite:blobs.db, fs:/path)
	RegistrationMode string  // RSTASH_REGISTRATION — "open" or "closed"
	LogLevel         string  // RSTASH_LOG_LEVEL — "debug", "info", "warn", "error"
	RateLimitRate    float64 // RSTASH_RATE_LIMIT — requests/sec per IP (0 = disabled)
	RateLimitBurst   int     // RSTASH_RATE_BURST — max burst size
	QuotaMode        string  // RSTASH_QUOTA_MODE — "off", "total", "user"
	QuotaTotal       int64   // RSTASH_QUOTA_TOTAL — bytes (parsed from human-readable)
	QuotaUser        int64   // RSTASH_QUOTA_USER — bytes (parsed from human-readable)
	MaxUploadSize    int64   // RSTASH_MAX_UPLOAD — max request body size (parsed from human-readable)
	TokenLifetime        string  // RSTASH_TOKEN_LIFETIME — OAuth token lifetime: "30d", "24h", "0" (no expiry)
	RefreshTokens        string  // "enabled" or "disabled"
	RefreshTokenLifetime string  // refresh token lifetime: "90d", "0" (no expiry)
	MetricsMode          string  // "public", "admin", or "off"
	PublicWrites         string  // "on" or "off"
	HomeSubtitle         string  // tagline on logged-out home page
	TOSMode              string  // "off", "text", "url"
	TOSContent           string
	PrivacyMode          string  // "off", "text", "url"
	PrivacyContent       string
	LogFile              string  // RSTASH_LOG_FILE — path to log file (empty = stderr only)
	TLSCert              string  // RSTASH_TLS_CERT — path to TLS certificate file
	TLSKey               string  // RSTASH_TLS_KEY — path to TLS private key file
	TLSMode              string  // RSTASH_TLS_MODE — "off", "manual", "auto", or "" (auto-detect)
	TLSCacheDir          string  // RSTASH_TLS_CACHE — autocert cache directory
	EmailDSN             string  // RSTASH_EMAIL — email provider DSN
}

// ParseDSN splits a DSN string into its scheme and path components.
// For example, "sqlite:rstash.db" returns ("sqlite", "rstash.db", nil).
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
		DatabaseDSN: envOrDefault(EnvDB, "sqlite:rstash.db"),
		BlobDSN:     envOrDefault(EnvBlob, "sqlite:rstash-blobs.db"),
		LogLevel:    envOrDefault(EnvLogLevel, "info"),
		LogFile:     os.Getenv(EnvLogFile),
		TLSCert:     os.Getenv(EnvTLSCert),
		TLSKey:      os.Getenv(EnvTLSKey),
		TLSMode:     os.Getenv(EnvTLSMode),
		TLSCacheDir: envOrDefault(EnvTLSCache, "./certs"),
		EmailDSN:    os.Getenv(EnvEmail),

		// Runtime-editable: sane defaults, changed via CLI/admin UI.
		HomeSubtitle:     "A personal remoteStorage server.",
		MetricsMode:      "public",
		PublicWrites:     "on",
		TOSMode:          "text",
		TOSContent:       defaultTOSContent,
		PrivacyMode:      "text",
		PrivacyContent:   defaultPrivacyContent,
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
		"log_file":          c.LogFile,
		"registration_mode": c.RegistrationMode,
		"log_level":         c.LogLevel,
		"rate_limit_rate":   fmt.Sprintf("%g", c.RateLimitRate),
		"rate_limit_burst":  fmt.Sprintf("%d", c.RateLimitBurst),
		"quota_mode":        c.QuotaMode,
		"quota_total":       FormatByteSize(c.QuotaTotal),
		"quota_user":        FormatByteSize(c.QuotaUser),
		"max_upload_size":   FormatByteSize(c.MaxUploadSize),
		"token_lifetime":         c.TokenLifetime,
		"refresh_tokens":         c.RefreshTokens,
		"refresh_token_lifetime": c.RefreshTokenLifetime,
		"tls_cert":               c.TLSCert,
		"tls_key":                c.TLSKey,
		"tls_mode":               c.TLSMode,
		"tls_cache":              c.TLSCacheDir,
		"email_dsn":              c.EmailDSN,
	}
}
