package storage

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// MIMEScanner rejects uploads whose detected MIME type matches a blocklist.
// It uses http.DetectContentType on the actual content (not the Content-Type header).
type MIMEScanner struct {
	// BlockedTypes returns the current comma-separated blocklist string.
	// Supports wildcards like "video/*".
	BlockedTypes func() string
}

// NewMIMEScanner creates a MIMEScanner that reads its blocklist dynamically.
func NewMIMEScanner(blockedTypes func() string) *MIMEScanner {
	return &MIMEScanner{BlockedTypes: blockedTypes}
}

func (s *MIMEScanner) Scan(_ context.Context, data []byte, _ string, _ int64, _ string) ScanResult {
	blocklist := s.BlockedTypes()
	if blocklist == "" {
		return ScanResult{Allowed: true}
	}

	detected := http.DetectContentType(data)
	// DetectContentType may return params like "text/html; charset=utf-8".
	if semi := strings.Index(detected, ";"); semi >= 0 {
		detected = strings.TrimSpace(detected[:semi])
	}

	for _, pattern := range strings.Split(blocklist, ",") {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if matchMIME(pattern, detected) {
			return ScanResult{
				Allowed: false,
				Reason:  fmt.Sprintf("content type %q is blocked by policy", detected),
			}
		}
	}

	return ScanResult{Allowed: true}
}

// matchMIME checks if a detected MIME type matches a pattern.
// Patterns can be exact ("application/pdf") or use wildcards ("video/*").
func matchMIME(pattern, detected string) bool {
	if pattern == detected {
		return true
	}
	// Wildcard: "video/*" matches "video/mp4".
	if strings.HasSuffix(pattern, "/*") {
		prefix := pattern[:len(pattern)-1] // "video/"
		return strings.HasPrefix(detected, prefix)
	}
	return false
}
