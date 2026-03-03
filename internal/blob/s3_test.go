package blob

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

func TestS3_ParseDSN(t *testing.T) {
	tests := []struct {
		name      string
		dsnPath   string
		wantCfg   s3Config
		wantErr   bool
		setEnv    map[string]string
		unsetEnv  []string
	}{
		{
			name:    "bucket only with defaults",
			dsnPath: "my-bucket",
			wantCfg: s3Config{
				Bucket:   "my-bucket",
				Region:   "us-east-1",
				Endpoint: "s3.amazonaws.com",
				UseTLS:   true,
			},
		},
		{
			name:    "all params specified",
			dsnPath: "my-bucket?region=eu-west-1&endpoint=custom.s3.com&prefix=data/prod&access_key=AK&secret_key=SK&tls=false",
			wantCfg: s3Config{
				Bucket:    "my-bucket",
				Region:    "eu-west-1",
				Endpoint:  "custom.s3.com",
				Prefix:    "data/prod",
				AccessKey: "AK",
				SecretKey: "SK",
				UseTLS:    false,
			},
		},
		{
			name:    "prefix with leading/trailing slashes trimmed",
			dsnPath: "bucket?prefix=/foo/bar/",
			wantCfg: s3Config{
				Bucket:   "bucket",
				Region:   "us-east-1",
				Endpoint: "s3.amazonaws.com",
				Prefix:   "foo/bar",
				UseTLS:   true,
			},
		},
		{
			name:    "env var fallback for credentials",
			dsnPath: "bucket?region=us-west-2",
			setEnv:  map[string]string{"AWS_ACCESS_KEY_ID": "envAK", "AWS_SECRET_ACCESS_KEY": "envSK"},
			wantCfg: s3Config{
				Bucket:    "bucket",
				Region:    "us-west-2",
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "envAK",
				SecretKey: "envSK",
				UseTLS:    true,
			},
		},
		{
			name:    "dsn credentials override env vars",
			dsnPath: "bucket?access_key=dsnAK&secret_key=dsnSK",
			setEnv:  map[string]string{"AWS_ACCESS_KEY_ID": "envAK", "AWS_SECRET_ACCESS_KEY": "envSK"},
			wantCfg: s3Config{
				Bucket:    "bucket",
				Region:    "us-east-1",
				Endpoint:  "s3.amazonaws.com",
				AccessKey: "dsnAK",
				SecretKey: "dsnSK",
				UseTLS:    true,
			},
		},
		{
			name:    "empty bucket",
			dsnPath: "",
			wantErr: true,
		},
		{
			name:    "invalid tls value",
			dsnPath: "bucket?tls=maybe",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env vars.
			for k, v := range tt.setEnv {
				old, existed := os.LookupEnv(k)
				os.Setenv(k, v)
				if existed {
					t.Cleanup(func() { os.Setenv(k, old) })
				} else {
					t.Cleanup(func() { os.Unsetenv(k) })
				}
			}
			for _, k := range tt.unsetEnv {
				old, existed := os.LookupEnv(k)
				os.Unsetenv(k)
				if existed {
					t.Cleanup(func() { os.Setenv(k, old) })
				}
			}

			cfg, err := parseS3DSN(tt.dsnPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Bucket != tt.wantCfg.Bucket {
				t.Errorf("Bucket = %q, want %q", cfg.Bucket, tt.wantCfg.Bucket)
			}
			if cfg.Region != tt.wantCfg.Region {
				t.Errorf("Region = %q, want %q", cfg.Region, tt.wantCfg.Region)
			}
			if cfg.Endpoint != tt.wantCfg.Endpoint {
				t.Errorf("Endpoint = %q, want %q", cfg.Endpoint, tt.wantCfg.Endpoint)
			}
			if cfg.Prefix != tt.wantCfg.Prefix {
				t.Errorf("Prefix = %q, want %q", cfg.Prefix, tt.wantCfg.Prefix)
			}
			if cfg.AccessKey != tt.wantCfg.AccessKey {
				t.Errorf("AccessKey = %q, want %q", cfg.AccessKey, tt.wantCfg.AccessKey)
			}
			if cfg.SecretKey != tt.wantCfg.SecretKey {
				t.Errorf("SecretKey = %q, want %q", cfg.SecretKey, tt.wantCfg.SecretKey)
			}
			if cfg.UseTLS != tt.wantCfg.UseTLS {
				t.Errorf("UseTLS = %v, want %v", cfg.UseTLS, tt.wantCfg.UseTLS)
			}
		})
	}
}

func TestS3_ObjectKey(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		userID int64
		path   string
		want   string
	}{
		{"no prefix", "", 42, "docs/hello.txt", "42/docs/hello.txt"},
		{"with prefix", "gosilo/prod", 7, "photos/cat.jpg", "gosilo/prod/7/photos/cat.jpg"},
		{"root path", "", 1, "file.bin", "1/file.bin"},
		{"leading slash stripped", "", 42, "/test/file.txt", "42/test/file.txt"},
		{"leading slash with prefix", "pfx", 3, "/a/b.txt", "pfx/3/a/b.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &S3Store{prefix: tt.prefix}
			got := s.objectKey(tt.userID, tt.path)
			if got != tt.want {
				t.Errorf("objectKey(%d, %q) = %q, want %q", tt.userID, tt.path, got, tt.want)
			}
		})
	}
}

// Integration tests — skipped unless GOSILO_TEST_S3_DSN is set.
// Example: GOSILO_TEST_S3_DSN="gosilo-test?endpoint=localhost:9000&tls=false&access_key=minioadmin&secret_key=minioadmin"

func newTestS3Store(t *testing.T) *S3Store {
	t.Helper()
	dsn := os.Getenv("GOSILO_TEST_S3_DSN")
	if dsn == "" {
		t.Skip("GOSILO_TEST_S3_DSN not set, skipping S3 integration test")
	}
	store, err := NewS3Store(dsn)
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestS3_Integration_PutGetRoundTrip(t *testing.T) {
	s := newTestS3Store(t)
	ctx := context.Background()

	data := []byte("hello, S3!")
	if err := s.Put(ctx, 1, "test/round-trip.txt", bytes.NewReader(data)); err != nil {
		t.Fatalf("Put: %v", err)
	}

	rc, err := s.Get(ctx, 1, "test/round-trip.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("data mismatch: got %q, want %q", got, data)
	}

	// Cleanup.
	s.Delete(ctx, 1, "test/round-trip.txt")
}

func TestS3_Integration_Overwrite(t *testing.T) {
	s := newTestS3Store(t)
	ctx := context.Background()

	if err := s.Put(ctx, 1, "test/overwrite.txt", bytes.NewReader([]byte("v1"))); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := s.Put(ctx, 1, "test/overwrite.txt", bytes.NewReader([]byte("v2"))); err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	rc, err := s.Get(ctx, 1, "test/overwrite.txt")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer rc.Close()

	got, _ := io.ReadAll(rc)
	if string(got) != "v2" {
		t.Errorf("got %q, want %q", got, "v2")
	}

	s.Delete(ctx, 1, "test/overwrite.txt")
}

func TestS3_Integration_Delete(t *testing.T) {
	s := newTestS3Store(t)
	ctx := context.Background()

	if err := s.Put(ctx, 1, "test/delete-me.txt", bytes.NewReader([]byte("bye"))); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Delete(ctx, 1, "test/delete-me.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := s.Get(ctx, 1, "test/delete-me.txt")
	if err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestS3_Integration_DeleteMissing(t *testing.T) {
	s := newTestS3Store(t)
	ctx := context.Background()

	// S3 DELETE is idempotent — deleting a non-existent key should not error.
	if err := s.Delete(ctx, 1, "test/nonexistent.txt"); err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}
}

func TestS3_Integration_GetMissing(t *testing.T) {
	s := newTestS3Store(t)
	ctx := context.Background()

	_, err := s.Get(ctx, 1, "test/does-not-exist.txt")
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestS3_Integration_DeleteTree(t *testing.T) {
	s := newTestS3Store(t)
	ctx := context.Background()

	// Create several files under a prefix.
	for _, name := range []string{"tree/a.txt", "tree/b.txt", "tree/sub/c.txt"} {
		if err := s.Put(ctx, 2, name, bytes.NewReader([]byte(name))); err != nil {
			t.Fatalf("Put %q: %v", name, err)
		}
	}

	if err := s.DeleteTree(ctx, 2, "tree/"); err != nil {
		t.Fatalf("DeleteTree: %v", err)
	}

	// All should be gone.
	for _, name := range []string{"tree/a.txt", "tree/b.txt", "tree/sub/c.txt"} {
		_, err := s.Get(ctx, 2, name)
		if err == nil {
			t.Errorf("expected %q to be deleted", name)
		}
	}
}

func TestS3_Integration_UserIsolation(t *testing.T) {
	s := newTestS3Store(t)
	ctx := context.Background()

	if err := s.Put(ctx, 10, "iso/file.txt", bytes.NewReader([]byte("user10"))); err != nil {
		t.Fatalf("Put user10: %v", err)
	}

	// User 11 should not see user 10's file.
	_, err := s.Get(ctx, 11, "iso/file.txt")
	if err == nil {
		t.Fatal("expected error for different user, got nil")
	}

	// Cleanup.
	s.Delete(ctx, 10, "iso/file.txt")
}
