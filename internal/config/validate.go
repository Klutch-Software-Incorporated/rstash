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
	} else if dbScheme != "sqlite" {
		errs = append(errs, fmt.Errorf("%s: only sqlite is supported for metadata database — got %q", EnvDB, dbScheme))
	}

	// BlobDSN
	if blobScheme, _, err := ParseDSN(c.BlobDSN); err != nil {
		errs = append(errs, fmt.Errorf("%s: %v", EnvBlob, err))
	} else {
		switch blobScheme {
		case "sqlite", "fs":
			// ok
		default:
			errs = append(errs, fmt.Errorf("%s: unsupported blob backend scheme %q (supported: sqlite, fs)", EnvBlob, blobScheme))
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

	// TLS: both or neither must be set.
	if (c.TLSCert != "") != (c.TLSKey != "") {
		errs = append(errs, fmt.Errorf("%s and %s must both be set or both be empty", EnvTLSCert, EnvTLSKey))
	}

	return errors.Join(errs...)
}
