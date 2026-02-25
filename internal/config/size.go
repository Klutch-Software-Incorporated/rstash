package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseByteSize parses a human-readable byte size string into bytes.
// Supported suffixes: B, KB, MB, GB, TB (binary: 1KB = 1024 bytes).
// Plain integers are treated as bytes. Returns 0 for "0".
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty size string")
	}

	upper := strings.ToUpper(s)

	type suffix struct {
		label      string
		multiplier int64
	}
	// Order matters: check longer suffixes first.
	suffixes := []suffix{
		{"TB", 1 << 40},
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
		{"B", 1},
	}

	for _, sf := range suffixes {
		if strings.HasSuffix(upper, sf.label) {
			numStr := strings.TrimSpace(s[:len(s)-len(sf.label)])
			if numStr == "" {
				return 0, fmt.Errorf("invalid size: %q", s)
			}
			n, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid size: %q", s)
			}
			if n < 0 {
				return 0, fmt.Errorf("negative size: %q", s)
			}
			return int64(n * float64(sf.multiplier)), nil
		}
	}

	// No suffix — treat as raw bytes.
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size: %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("negative size: %q", s)
	}
	return n, nil
}

// ParseTokenLifetime parses a token lifetime string into a time.Duration.
// Supported formats: Go duration strings (e.g. "720h", "30m"), plus a "d" day
// suffix for convenience (e.g. "30d" = 30*24h). "0" means no expiry.
// Returns 0 for no expiry.
func ParseTokenLifetime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}

	// Custom "d" suffix: convert days to hours and delegate to time.ParseDuration.
	lower := strings.ToLower(s)
	if strings.HasSuffix(lower, "d") {
		numStr := strings.TrimSpace(s[:len(s)-1])
		n, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid token lifetime: %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid token lifetime: %q", s)
	}
	if d < 0 {
		return 0, fmt.Errorf("negative token lifetime: %q", s)
	}
	return d, nil
}

// FormatByteSize returns a human-readable string for a byte count.
func FormatByteSize(b int64) string {
	switch {
	case b >= 1<<40:
		return fmt.Sprintf("%.1f TB", float64(b)/float64(1<<40))
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
