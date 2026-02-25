package config

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Validate checks all configuration values and returns an error describing
// every problem found. It normalizes BaseURL by stripping a trailing slash.
func (c *Config) Validate() error {
	var errs []error

	// Addr
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		errs = append(errs, fmt.Errorf("GOSILO_ADDR: invalid host:port — %v", err))
	}

	// BaseURL
	if u, err := url.Parse(c.BaseURL); err != nil {
		errs = append(errs, fmt.Errorf("GOSILO_BASE_URL: invalid URL — %v", err))
	} else if u.Scheme != "http" && u.Scheme != "https" {
		errs = append(errs, fmt.Errorf("GOSILO_BASE_URL: scheme must be http or https — got %q", u.Scheme))
	} else if u.Host == "" {
		errs = append(errs, fmt.Errorf("GOSILO_BASE_URL: host must not be empty"))
	} else {
		// Normalize: strip trailing slash.
		c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	}

	// DatabaseDSN
	if dbScheme, _, err := ParseDSN(c.DatabaseDSN); err != nil {
		errs = append(errs, fmt.Errorf("GOSILO_DB: %v", err))
	} else if dbScheme != "sqlite" {
		errs = append(errs, fmt.Errorf("GOSILO_DB: only sqlite is supported for metadata database — got %q", dbScheme))
	}

	// BlobDSN
	if blobScheme, _, err := ParseDSN(c.BlobDSN); err != nil {
		errs = append(errs, fmt.Errorf("GOSILO_BLOB: %v", err))
	} else {
		switch blobScheme {
		case "sqlite", "fs":
			// ok
		default:
			errs = append(errs, fmt.Errorf("GOSILO_BLOB: unsupported blob backend scheme %q (supported: sqlite, fs)", blobScheme))
		}
	}

	// RegistrationMode
	switch c.RegistrationMode {
	case "open", "closed":
		// ok
	default:
		errs = append(errs, fmt.Errorf("GOSILO_REGISTRATION: must be one of: open, closed — got %q", c.RegistrationMode))
	}

	// LogLevel
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// ok
	default:
		errs = append(errs, fmt.Errorf("GOSILO_LOG_LEVEL: must be one of: debug, info, warn, error — got %q", c.LogLevel))
	}

	// RateLimitRate
	if c.RateLimitRate < 0 {
		errs = append(errs, fmt.Errorf("GOSILO_RATE_LIMIT: must be >= 0 — got %v", c.RateLimitRate))
	}

	// RateLimitBurst (only relevant when rate limiting is enabled)
	if c.RateLimitRate > 0 && c.RateLimitBurst < 1 {
		errs = append(errs, fmt.Errorf("GOSILO_RATE_BURST: must be >= 1 when rate limiting is enabled — got %d", c.RateLimitBurst))
	}

	// QuotaMode
	switch c.QuotaMode {
	case "off", "total", "user":
		// ok
	default:
		errs = append(errs, fmt.Errorf("GOSILO_QUOTA_MODE: must be one of: off, total, user — got %q", c.QuotaMode))
	}

	// QuotaTotal required and > 0 when mode=total
	if c.QuotaMode == "total" && c.QuotaTotal <= 0 {
		errs = append(errs, fmt.Errorf("GOSILO_QUOTA_TOTAL: must be > 0 when GOSILO_QUOTA_MODE=total"))
	}

	// QuotaUser required and > 0 when mode=user
	if c.QuotaMode == "user" && c.QuotaUser <= 0 {
		errs = append(errs, fmt.Errorf("GOSILO_QUOTA_USER: must be > 0 when GOSILO_QUOTA_MODE=user"))
	}

	// MaxUploadSize
	if c.MaxUploadSize <= 0 {
		errs = append(errs, fmt.Errorf("GOSILO_MAX_UPLOAD: must be > 0"))
	}

	// WebMode
	switch c.WebMode {
	case "full", "oauth", "off":
		// ok
	default:
		errs = append(errs, fmt.Errorf("GOSILO_WEB_MODE: must be one of: full, oauth, off — got %q", c.WebMode))
	}

	// TLS: both or neither must be set.
	if (c.TLSCert != "") != (c.TLSKey != "") {
		errs = append(errs, fmt.Errorf("GOSILO_TLS_CERT and GOSILO_TLS_KEY must both be set or both be empty"))
	}

	return errors.Join(errs...)
}
