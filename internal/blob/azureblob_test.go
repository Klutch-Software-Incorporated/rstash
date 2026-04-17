package blob

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"
)

func TestAzureBlob_ParseDSN(t *testing.T) {
	tests := []struct {
		name    string
		dsnPath string
		wantCfg azureBlobConfig
		wantErr bool
	}{
		{
			name:    "container with account uses default endpoint and TLS",
			dsnPath: "rstash?account=myaccount",
			wantCfg: azureBlobConfig{
				Container:   "rstash",
				AccountName: "myaccount",
				Endpoint:    "myaccount.blob.core.windows.net",
				UseTLS:      true,
			},
		},
		{
			name:    "all params specified",
			dsnPath: "mycontainer?account=acct&key=base64key==&prefix=data/prod&tls=false&endpoint=127.0.0.1:10000",
			wantCfg: azureBlobConfig{
				Container:   "mycontainer",
				AccountName: "acct",
				Key:         "base64key==",
				Prefix:      "data/prod",
				Endpoint:    "127.0.0.1:10000",
				UseTLS:      false,
			},
		},
		{
			name:    "SAS token",
			dsnPath: "c?account=acct&sas=sv%3D2022-11-02%26sig%3Dabc",
			wantCfg: azureBlobConfig{
				Container:   "c",
				AccountName: "acct",
				SASToken:    "sv=2022-11-02&sig=abc",
				Endpoint:    "acct.blob.core.windows.net",
				UseTLS:      true,
			},
		},
		{
			name:    "prefix slashes trimmed",
			dsnPath: "c?account=a&prefix=/foo/bar/",
			wantCfg: azureBlobConfig{
				Container:   "c",
				AccountName: "a",
				Prefix:      "foo/bar",
				Endpoint:    "a.blob.core.windows.net",
				UseTLS:      true,
			},
		},
		{
			name:    "empty container",
			dsnPath: "?account=a",
			wantErr: true,
		},
		{
			name:    "missing account",
			dsnPath: "my-container",
			wantErr: true,
		},
		{
			name:    "empty account value",
			dsnPath: "c?account=",
			wantErr: true,
		},
		{
			name:    "invalid tls value",
			dsnPath: "c?account=a&tls=maybe",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := parseAzureBlobDSN(tt.dsnPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Container != tt.wantCfg.Container {
				t.Errorf("Container = %q, want %q", cfg.Container, tt.wantCfg.Container)
			}
			if cfg.AccountName != tt.wantCfg.AccountName {
				t.Errorf("AccountName = %q, want %q", cfg.AccountName, tt.wantCfg.AccountName)
			}
			if cfg.Key != tt.wantCfg.Key {
				t.Errorf("Key = %q, want %q", cfg.Key, tt.wantCfg.Key)
			}
			if cfg.SASToken != tt.wantCfg.SASToken {
				t.Errorf("SASToken = %q, want %q", cfg.SASToken, tt.wantCfg.SASToken)
			}
			if cfg.Endpoint != tt.wantCfg.Endpoint {
				t.Errorf("Endpoint = %q, want %q", cfg.Endpoint, tt.wantCfg.Endpoint)
			}
			if cfg.Prefix != tt.wantCfg.Prefix {
				t.Errorf("Prefix = %q, want %q", cfg.Prefix, tt.wantCfg.Prefix)
			}
			if cfg.UseTLS != tt.wantCfg.UseTLS {
				t.Errorf("UseTLS = %v, want %v", cfg.UseTLS, tt.wantCfg.UseTLS)
			}
		})
	}
}

func TestAzureBlob_BlobName(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		userID int64
		path   string
		want   string
	}{
		{"no prefix", "", 42, "docs/hello.txt", "42/docs/hello.txt"},
		{"with prefix", "rstash/prod", 7, "photos/cat.jpg", "rstash/prod/7/photos/cat.jpg"},
		{"root path", "", 1, "file.bin", "1/file.bin"},
		{"leading slash stripped", "", 42, "/test/file.txt", "42/test/file.txt"},
		{"leading slash with prefix", "pfx", 3, "/a/b.txt", "pfx/3/a/b.txt"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &AzureBlobStore{prefix: tt.prefix}
			got := s.blobName(tt.userID, tt.path)
			if got != tt.want {
				t.Errorf("blobName(%d, %q) = %q, want %q", tt.userID, tt.path, got, tt.want)
			}
		})
	}
}

// Integration tests — skipped unless RSTASH_TEST_AZURE_BLOB_DSN is set.
// Example (Azurite): RSTASH_TEST_AZURE_BLOB_DSN="rstash-test?account=devstoreaccount1&key=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==&endpoint=127.0.0.1:10000&tls=false"

func newTestAzureBlobStore(t *testing.T) *AzureBlobStore {
	t.Helper()
	dsn := os.Getenv("RSTASH_TEST_AZURE_BLOB_DSN")
	if dsn == "" {
		t.Skip("RSTASH_TEST_AZURE_BLOB_DSN not set, skipping Azure Blob integration test")
	}
	store, err := NewAzureBlobStore(dsn)
	if err != nil {
		t.Fatalf("NewAzureBlobStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestAzureBlob_Integration_PutGetRoundTrip(t *testing.T) {
	s := newTestAzureBlobStore(t)
	ctx := context.Background()

	data := []byte("hello, Azure!")
	if err := s.Put(ctx, 1, "test/round-trip.txt", data); err != nil {
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

	s.Delete(ctx, 1, "test/round-trip.txt")
}

func TestAzureBlob_Integration_Overwrite(t *testing.T) {
	s := newTestAzureBlobStore(t)
	ctx := context.Background()

	if err := s.Put(ctx, 1, "test/overwrite.txt", []byte("v1")); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := s.Put(ctx, 1, "test/overwrite.txt", []byte("v2")); err != nil {
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

func TestAzureBlob_Integration_DeleteMissing(t *testing.T) {
	s := newTestAzureBlobStore(t)
	ctx := context.Background()

	// Delete of a non-existent blob should not error — same as S3 behavior.
	if err := s.Delete(ctx, 1, "test/nonexistent.txt"); err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}
}

func TestAzureBlob_Integration_GetMissing(t *testing.T) {
	s := newTestAzureBlobStore(t)
	ctx := context.Background()

	_, err := s.Get(ctx, 1, "test/does-not-exist.txt")
	if err == nil {
		t.Fatal("expected error for missing blob, got nil")
	}
}

func TestAzureBlob_Integration_DeleteTree(t *testing.T) {
	s := newTestAzureBlobStore(t)
	ctx := context.Background()

	for _, name := range []string{"tree/a.txt", "tree/b.txt", "tree/sub/c.txt"} {
		if err := s.Put(ctx, 2, name, []byte(name)); err != nil {
			t.Fatalf("Put %q: %v", name, err)
		}
	}

	if err := s.DeleteTree(ctx, 2, "tree/"); err != nil {
		t.Fatalf("DeleteTree: %v", err)
	}

	for _, name := range []string{"tree/a.txt", "tree/b.txt", "tree/sub/c.txt"} {
		_, err := s.Get(ctx, 2, name)
		if err == nil {
			t.Errorf("expected %q to be deleted", name)
		}
	}
}

func TestAzureBlob_Integration_UserIsolation(t *testing.T) {
	s := newTestAzureBlobStore(t)
	ctx := context.Background()

	if err := s.Put(ctx, 10, "iso/file.txt", []byte("user10")); err != nil {
		t.Fatalf("Put user10: %v", err)
	}

	_, err := s.Get(ctx, 11, "iso/file.txt")
	if err == nil {
		t.Fatal("expected error for different user, got nil")
	}

	s.Delete(ctx, 10, "iso/file.txt")
}
