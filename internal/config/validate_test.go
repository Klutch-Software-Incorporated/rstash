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
		DatabaseDSN:      "sqlite:rstash.sqlite",
		BlobDSN:          "sqlite:rstash-blobs.sqlite",
		RegistrationMode: "closed",
		LogLevel:         "info",
		RateLimitRate:    10,
		RateLimitBurst:   20,
		QuotaMode:        "off",
		MaxUploadSize:    50 << 20, // 50MB
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
	if !strings.Contains(err.Error(), "RSTASH_ADDR") {
		t.Fatalf("error should mention RSTASH_ADDR, got: %v", err)
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
		{"empty scheme", "://example.com", "RSTASH_BASE_URL"},
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

func TestValidate_DatabaseDSN(t *testing.T) {
	// Valid DSNs for all supported databases.
	for _, dsn := range []string{
		"sqlite:rstash.sqlite",
		"sqlite::memory:",
		"postgres:host=localhost dbname=rstash",
		"mysql:user:pass@tcp(localhost:3306)/rstash",
		"mssql:sqlserver://sa:pass@localhost:1433?database=rstash",
	} {
		cfg := validConfig()
		cfg.DatabaseDSN = dsn
		if err := cfg.Validate(); err != nil {
			t.Errorf("database DSN %q should be valid, got: %v", dsn, err)
		}
	}

	// Unsupported scheme.
	cfg := validConfig()
	cfg.DatabaseDSN = "redis:localhost:6379"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported database DSN scheme")
	}
	if !strings.Contains(err.Error(), "RSTASH_DB") {
		t.Fatalf("error should mention RSTASH_DB, got: %v", err)
	}
}

func TestValidate_BlobDSN(t *testing.T) {
	for _, dsn := range []string{"sqlite:blobs.db", "fs:/tmp/blobs", "s3:mybucket", "azureblob:mycontainer?account=myacct"} {
		cfg := validConfig()
		cfg.BlobDSN = dsn
		if err := cfg.Validate(); err != nil {
			t.Errorf("blob DSN %q should be valid, got: %v", dsn, err)
		}
	}

	cfg := validConfig()
	cfg.BlobDSN = "redis:localhost:6379"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for unsupported blob backend scheme")
	}
	if !strings.Contains(err.Error(), "RSTASH_BLOB") {
		t.Fatalf("error should mention RSTASH_BLOB, got: %v", err)
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

func TestValidate_TLS(t *testing.T) {
	// Both set — valid.
	cfg := validConfig()
	cfg.TLSCert = "/tmp/cert.pem"
	cfg.TLSKey = "/tmp/key.pem"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("both TLS fields set should be valid, got: %v", err)
	}

	// Only cert — invalid.
	cfg = validConfig()
	cfg.TLSCert = "/tmp/cert.pem"
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when only TLS cert is set")
	}
	if !strings.Contains(err.Error(), "RSTASH_TLS_CERT") {
		t.Fatalf("error should mention TLS, got: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Addr:        "bad",
		BaseURL:     "ftp://x.com",
		DatabaseDSN: "redis:localhost",
		BlobDSN:     "redis:localhost",
		LogLevel:    "trace",
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected multiple errors")
	}
	msg := err.Error()
	for _, want := range []string{"RSTASH_ADDR", "RSTASH_BASE_URL", "RSTASH_DB", "RSTASH_BLOB", "RSTASH_LOG_LEVEL"} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q, got: %v", want, msg)
		}
	}
}
