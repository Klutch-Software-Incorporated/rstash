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
//
// Only env-var-backed settings are validated here — runtime-editable settings
// use hardcoded defaults that are always valid, and are validated on write
// by the settings package.
func (c *Config) Validate() error {
	var errs []error

	// Addr
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		errs = append(errs, fmt.Errorf("%s: invalid host:port — %v", EnvAddr, err))
	}

	// BaseURL
	if u, err := url.Parse(c.BaseURL); err != nil {
		errs = append(errs, fmt.Errorf("%s: invalid URL — %v", EnvBaseURL, err))
	} else if u.Scheme != "http" && u.Scheme != "https" {
		errs = append(errs, fmt.Errorf("%s: scheme must be http or https — got %q", EnvBaseURL, u.Scheme))
	} else if u.Host == "" {
		errs = append(errs, fmt.Errorf("%s: host must not be empty", EnvBaseURL))
	} else {
		// Normalize: strip trailing slash.
		c.BaseURL = strings.TrimRight(c.BaseURL, "/")
	}

	// DatabaseDSN
	if dbScheme, _, err := ParseDSN(c.DatabaseDSN); err != nil {
		errs = append(errs, fmt.Errorf("%s: %v", EnvDB, err))
	} else {
		switch dbScheme {
		case "sqlite", "postgres", "mysql", "mssql":
			// ok
		default:
			errs = append(errs, fmt.Errorf("%s: unsupported database scheme %q (supported: sqlite, postgres, mysql, mssql)", EnvDB, dbScheme))
		}
	}

	// BlobDSN
	if blobScheme, _, err := ParseDSN(c.BlobDSN); err != nil {
		errs = append(errs, fmt.Errorf("%s: %v", EnvBlob, err))
	} else {
		switch blobScheme {
		case "sqlite", "fs", "postgres", "mysql", "mssql", "s3":
			// ok
		default:
			errs = append(errs, fmt.Errorf("%s: unsupported blob backend scheme %q (supported: sqlite, fs, postgres, mysql, mssql, s3)", EnvBlob, blobScheme))
		}
	}

	// LogLevel
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// ok
	default:
		errs = append(errs, fmt.Errorf("%s: must be one of: debug, info, warn, error — got %q", EnvLogLevel, c.LogLevel))
	}

	// WebMode
	switch c.WebMode {
	case "full", "oauth", "off":
		// ok
	default:
		errs = append(errs, fmt.Errorf("%s: must be one of: full, oauth, off — got %q", EnvWebMode, c.WebMode))
	}

	// TLS mode-aware validation.
	switch c.TLSMode {
	case "manual":
		if c.TLSCert == "" || c.TLSKey == "" {
			errs = append(errs, fmt.Errorf("%s=manual requires both %s and %s", EnvTLSMode, EnvTLSCert, EnvTLSKey))
		}
	case "auto":
		if c.TLSCert != "" || c.TLSKey != "" {
			errs = append(errs, fmt.Errorf("%s=auto conflicts with %s/%s — remove cert/key settings", EnvTLSMode, EnvTLSCert, EnvTLSKey))
		}
		if u, err := url.Parse(c.BaseURL); err == nil && u.Scheme != "https" {
			errs = append(errs, fmt.Errorf("%s=auto requires %s to use https scheme", EnvTLSMode, EnvBaseURL))
		}
		if u, err := url.Parse(c.BaseURL); err == nil {
			host := u.Hostname()
			if host == "localhost" || host == "127.0.0.1" || host == "::1" {
				errs = append(errs, fmt.Errorf("%s=auto cannot use localhost — Let's Encrypt requires a public hostname", EnvTLSMode))
			}
		}
	case "off":
		if c.TLSCert != "" || c.TLSKey != "" {
			errs = append(errs, fmt.Errorf("%s=off conflicts with %s/%s — remove cert/key settings or change mode", EnvTLSMode, EnvTLSCert, EnvTLSKey))
		}
	case "":
		// Auto-detect: existing "both or neither" logic.
		if (c.TLSCert != "") != (c.TLSKey != "") {
			errs = append(errs, fmt.Errorf("%s and %s must both be set or both be empty", EnvTLSCert, EnvTLSKey))
		}
	default:
		errs = append(errs, fmt.Errorf("%s: must be one of: off, manual, auto — got %q", EnvTLSMode, c.TLSMode))
	}

	return errors.Join(errs...)
}
