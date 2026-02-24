package config

import (
	"strings"
	"testing"
)

// validConfig returns a Config that passes all validation.
func validConfig() *Config {
	return &Config{
		Addr:             ":8080",
		BaseURL:          "http://localhost:8080",
		DatabasePath:     "gosilo.db",
		BlobBackend:      "sqlite",
		BlobPath:         "",
		RegistrationMode: "closed",
		LogLevel:         "info",
		RateLimitRate:    10,
		RateLimitBurst:   20,
		QuotaMode:        "off",
	}
}

func TestValidate_DefaultConfig(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid, got: %v", err)
	}
}

func TestValidate_InvalidAddr(t *testing.T) {
	cfg := validConfig()
	cfg.Addr = "no-port"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid addr")
	}
	if !strings.Contains(err.Error(), "GOSILO_ADDR") {
		t.Fatalf("error should mention GOSILO_ADDR, got: %v", err)
	}
}

func TestValidate_ValidAddrs(t *testing.T) {
	for _, addr := range []string{":8080", "0.0.0.0:8080", "127.0.0.1:443", "[::1]:8080"} {
		cfg := validConfig()
		cfg.Addr = addr
		if err := cfg.Validate(); err != nil {
			t.Errorf("addr %q should be valid, got: %v", addr, err)
		}
	}
}

func TestValidate_BaseURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"valid http", "http://example.com", ""},
		{"valid https", "https://example.com", ""},
		{"ftp scheme", "ftp://example.com", "scheme must be http or https"},
		{"empty scheme", "://example.com", "GOSILO_BASE_URL"},
		{"empty host", "http://", "host must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.BaseURL = tt.url
			err := cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}

func TestValidate_BaseURL_TrailingSlash(t *testing.T) {
	cfg := validConfig()
	cfg.BaseURL = "http://example.com/"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("trailing slash should be normalized, got error: %v", err)
	}
	if cfg.BaseURL != "http://example.com" {
		t.Fatalf("expected trailing slash stripped, got %q", cfg.BaseURL)
	}
}

func TestValidate_EmptyDatabasePath(t *testing.T) {
	cfg := validConfig()
	cfg.DatabasePath = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty database path")
	}
	if !strings.Contains(err.Error(), "GOSILO_DB_PATH") {
		t.Fatalf("error should mention GOSILO_DB_PATH, got: %v", err)
	}
}

func TestValidate_BlobBackend(t *testing.T) {
	for _, backend := range []string{"sqlite", "fs"} {
		cfg := validConfig()
		cfg.BlobBackend = backend
		if backend == "fs" {
			cfg.BlobPath = "/tmp/blobs"
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("backend %q should be valid, got: %v", backend, err)
		}
	}

	cfg := validConfig()
	cfg.BlobBackend = "s3"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid blob backend")
	}
	if !strings.Contains(err.Error(), `got "s3"`) {
		t.Fatalf("error should mention s3, got: %v", err)
	}
}

func TestValidate_BlobPath_RequiredForFS(t *testing.T) {
	cfg := validConfig()
	cfg.BlobBackend = "fs"
	cfg.BlobPath = ""
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for fs backend without blob path")
	}
	if !strings.Contains(err.Error(), "GOSILO_BLOB_PATH") {
		t.Fatalf("error should mention GOSILO_BLOB_PATH, got: %v", err)
	}
}

func TestValidate_BlobPath_NotRequiredForSQLite(t *testing.T) {
	cfg := validConfig()
	cfg.BlobBackend = "sqlite"
	cfg.BlobPath = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("blob path should not be required for sqlite, got: %v", err)
	}
}

func TestValidate_RegistrationMode(t *testing.T) {
	for _, mode := range []string{"open", "invite", "closed"} {
		cfg := validConfig()
		cfg.RegistrationMode = mode
		if err := cfg.Validate(); err != nil {
			t.Errorf("registration mode %q should be valid, got: %v", mode, err)
		}
	}

	cfg := validConfig()
	cfg.RegistrationMode = "whatever"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid registration mode")
	}
	if !strings.Contains(err.Error(), `got "whatever"`) {
		t.Fatalf("error should mention invalid value, got: %v", err)
	}
}

func TestValidate_LogLevel(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		cfg := validConfig()
		cfg.LogLevel = level
		if err := cfg.Validate(); err != nil {
			t.Errorf("log level %q should be valid, got: %v", level, err)
		}
	}

	cfg := validConfig()
	cfg.LogLevel = "trace"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	if !strings.Contains(err.Error(), `got "trace"`) {
		t.Fatalf("error should mention trace, got: %v", err)
	}
}

func TestValidate_RateLimitRate_Negative(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimitRate = -1
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative rate limit")
	}
	if !strings.Contains(err.Error(), "GOSILO_RATE_LIMIT") {
		t.Fatalf("error should mention GOSILO_RATE_LIMIT, got: %v", err)
	}
}

func TestValidate_RateLimitRate_ZeroDisables(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimitRate = 0
	cfg.RateLimitBurst = 0 // burst doesn't matter when disabled
	if err := cfg.Validate(); err != nil {
		t.Fatalf("zero rate should be valid (disables limiting), got: %v", err)
	}
}

func TestValidate_RateLimitBurst_InvalidWhenEnabled(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimitRate = 10
	cfg.RateLimitBurst = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for zero burst with rate > 0")
	}
	if !strings.Contains(err.Error(), "GOSILO_RATE_BURST") {
		t.Fatalf("error should mention GOSILO_RATE_BURST, got: %v", err)
	}
}

func TestValidate_RateLimitBurst_ValidWhenEnabled(t *testing.T) {
	cfg := validConfig()
	cfg.RateLimitRate = 5
	cfg.RateLimitBurst = 1
	if err := cfg.Validate(); err != nil {
		t.Fatalf("burst=1 with rate>0 should be valid, got: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Addr:             "bad",
		BaseURL:          "ftp://x.com",
		DatabasePath:     "",
		BlobBackend:      "s3",
		RegistrationMode: "maybe",
		LogLevel:         "trace",
		QuotaMode:        "off",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected multiple errors")
	}
	msg := err.Error()
	for _, want := range []string{"GOSILO_ADDR", "GOSILO_BASE_URL", "GOSILO_DB_PATH", "GOSILO_BLOB_BACKEND", "GOSILO_REGISTRATION", "GOSILO_LOG_LEVEL"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q, got: %v", want, msg)
		}
	}
}

func TestValidate_QuotaMode(t *testing.T) {
	for _, mode := range []string{"off", "total", "user"} {
		cfg := validConfig()
		cfg.QuotaMode = mode
		if mode == "total" {
			cfg.QuotaTotal = 1024
		}
		if mode == "user" {
			cfg.QuotaUser = 1024
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("quota mode %q should be valid, got: %v", mode, err)
		}
	}

	cfg := validConfig()
	cfg.QuotaMode = "invalid"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid quota mode")
	}
	if !strings.Contains(err.Error(), "GOSILO_QUOTA_MODE") {
		t.Fatalf("error should mention GOSILO_QUOTA_MODE, got: %v", err)
	}
}

func TestValidate_QuotaTotal_RequiredForTotalMode(t *testing.T) {
	cfg := validConfig()
	cfg.QuotaMode = "total"
	cfg.QuotaTotal = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for total mode without quota total")
	}
	if !strings.Contains(err.Error(), "GOSILO_QUOTA_TOTAL") {
		t.Fatalf("error should mention GOSILO_QUOTA_TOTAL, got: %v", err)
	}
}

func TestValidate_QuotaUser_RequiredForUserMode(t *testing.T) {
	cfg := validConfig()
	cfg.QuotaMode = "user"
	cfg.QuotaUser = 0
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for user mode without quota user")
	}
	if !strings.Contains(err.Error(), "GOSILO_QUOTA_USER") {
		t.Fatalf("error should mention GOSILO_QUOTA_USER, got: %v", err)
	}
}

func TestValidate_QuotaOff_NoLimitsNeeded(t *testing.T) {
	cfg := validConfig()
	cfg.QuotaMode = "off"
	cfg.QuotaTotal = 0
	cfg.QuotaUser = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("quota off should not require limits, got: %v", err)
	}
}
