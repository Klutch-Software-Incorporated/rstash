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

	// DatabasePath
	if c.DatabasePath == "" {
		errs = append(errs, fmt.Errorf("GOSILO_DB_PATH: must not be empty"))
	}

	// BlobBackend
	switch c.BlobBackend {
	case "sqlite", "fs":
		// ok
	default:
		errs = append(errs, fmt.Errorf("GOSILO_BLOB_BACKEND: must be one of: sqlite, fs — got %q", c.BlobBackend))
	}

	// BlobPath required when backend is fs
	if c.BlobBackend == "fs" && c.BlobPath == "" {
		errs = append(errs, fmt.Errorf("GOSILO_BLOB_PATH: required when GOSILO_BLOB_BACKEND=fs"))
	}

	// RegistrationMode
	switch c.RegistrationMode {
	case "open", "invite", "closed":
		// ok
	default:
		errs = append(errs, fmt.Errorf("GOSILO_REGISTRATION: must be one of: open, invite, closed — got %q", c.RegistrationMode))
	}

	// LogLevel
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
		// ok
	default:
		errs = append(errs, fmt.Errorf("GOSILO_LOG_LEVEL: must be one of: debug, info, warn, error — got %q", c.LogLevel))
	}

	return errors.Join(errs...)
}
