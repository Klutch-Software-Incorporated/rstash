package storage

import (
	"context"
	"testing"
)

func TestMIMEScanner_EmptyBlocklist(t *testing.T) {
	s := NewMIMEScanner(func() string { return "" })
	result := s.Scan(context.Background(), []byte("hello world"), "text/plain", 1, "/test.txt")
	if !result.Allowed {
		t.Fatal("empty blocklist should allow everything")
	}
}

func TestMIMEScanner_ExactMatch(t *testing.T) {
	s := NewMIMEScanner(func() string { return "text/html" })

	// HTML content should be blocked.
	html := []byte("<html><body>hello</body></html>")
	result := s.Scan(context.Background(), html, "text/html", 1, "/test.html")
	if result.Allowed {
		t.Fatal("text/html should be blocked")
	}
	if result.Reason == "" {
		t.Fatal("expected a reason for rejection")
	}

	// Plain text should pass.
	result = s.Scan(context.Background(), []byte("hello world"), "text/plain", 1, "/test.txt")
	if !result.Allowed {
		t.Fatal("text/plain should be allowed when only text/html is blocked")
	}
}

func TestMIMEScanner_WildcardMatch(t *testing.T) {
	// DetectContentType won't detect video from arbitrary bytes,
	// so we test the matchMIME logic directly.
	if !matchMIME("video/*", "video/mp4") {
		t.Fatal("video/* should match video/mp4")
	}
	if !matchMIME("video/*", "video/webm") {
		t.Fatal("video/* should match video/webm")
	}
	if matchMIME("video/*", "audio/mp3") {
		t.Fatal("video/* should not match audio/mp3")
	}
}

func TestMIMEScanner_MultiplePatterns(t *testing.T) {
	s := NewMIMEScanner(func() string { return "text/html, application/pdf" })

	html := []byte("<html><body>hello</body></html>")
	result := s.Scan(context.Background(), html, "", 1, "/test.html")
	if result.Allowed {
		t.Fatal("text/html should be blocked")
	}

	// PDF magic bytes.
	pdf := []byte("%PDF-1.4 fake pdf content")
	result = s.Scan(context.Background(), pdf, "", 1, "/test.pdf")
	if result.Allowed {
		t.Fatal("application/pdf should be blocked")
	}
}

func TestMIMEScanner_DynamicBlocklist(t *testing.T) {
	blocked := ""
	s := NewMIMEScanner(func() string { return blocked })

	html := []byte("<html><body>hello</body></html>")

	// Initially allowed.
	result := s.Scan(context.Background(), html, "", 1, "/test.html")
	if !result.Allowed {
		t.Fatal("should be allowed with empty blocklist")
	}

	// Dynamically block.
	blocked = "text/html"
	result = s.Scan(context.Background(), html, "", 1, "/test.html")
	if result.Allowed {
		t.Fatal("should be blocked after updating blocklist")
	}
}

func TestMatchMIME(t *testing.T) {
	tests := []struct {
		pattern, detected string
		want              bool
	}{
		{"text/html", "text/html", true},
		{"text/html", "text/plain", false},
		{"video/*", "video/mp4", true},
		{"video/*", "video/webm", true},
		{"video/*", "audio/mp3", false},
		{"image/*", "image/png", true},
		{"image/*", "image/jpeg", true},
		{"image/*", "text/plain", false},
		{"application/pdf", "application/pdf", true},
		{"application/pdf", "application/json", false},
	}

	for _, tt := range tests {
		got := matchMIME(tt.pattern, tt.detected)
		if got != tt.want {
			t.Errorf("matchMIME(%q, %q) = %v, want %v", tt.pattern, tt.detected, got, tt.want)
		}
	}
}
