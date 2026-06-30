package storage

import "testing"

func TestDocumentETag(t *testing.T) {
	etag := DocumentETag([]byte("hello"))
	if len(etag) != 16 {
		t.Fatalf("expected 16 hex chars, got %d: %q", len(etag), etag)
	}

	// Same content must produce the same ETag.
	etag2 := DocumentETag([]byte("hello"))
	if etag != etag2 {
		t.Fatalf("expected same etag, got %q vs %q", etag, etag2)
	}

	// Different content must produce a different ETag.
	etag3 := DocumentETag([]byte("world"))
	if etag == etag3 {
		t.Fatalf("expected different etags for different content")
	}
}

func TestFolderETag(t *testing.T) {
	children := map[string]string{
		"a.txt": "abc",
		"b.txt": "def",
	}
	etag := FolderETag(children)
	if len(etag) != 16 {
		t.Fatalf("expected 16 hex chars, got %d: %q", len(etag), etag)
	}

	// Same children must produce the same ETag.
	etag2 := FolderETag(children)
	if etag != etag2 {
		t.Fatalf("expected same etag, got %q vs %q", etag, etag2)
	}

	// Empty folder.
	empty := FolderETag(map[string]string{})
	if len(empty) != 16 {
		t.Fatalf("expected 16 hex chars for empty folder, got %d: %q", len(empty), empty)
	}
}

func TestQuoteUnquoteETag(t *testing.T) {
	quoted := QuoteETag("abc123")
	if quoted != `"abc123"` {
		t.Fatalf("expected %q, got %q", `"abc123"`, quoted)
	}

	unquoted := UnquoteETag(`"abc123"`)
	if unquoted != "abc123" {
		t.Fatalf("expected %q, got %q", "abc123", unquoted)
	}

	// Already unquoted.
	bare := UnquoteETag("abc123")
	if bare != "abc123" {
		t.Fatalf("expected %q, got %q", "abc123", bare)
	}
}
