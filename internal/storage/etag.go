package storage

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
)

// DocumentETag returns the ETag for a document: first 16 hex chars of SHA-256.
func DocumentETag(content []byte) string {
	h := sha256.Sum256(content)
	return fmt.Sprintf("%x", h[:8])
}

// FolderETag returns the ETag for a folder based on its direct children.
// It hashes the sorted name+etag pairs of all children.
func FolderETag(children map[string]string) string {
	names := make([]string, 0, len(children))
	for name := range children {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		b.WriteString(children[name])
	}

	h := sha256.Sum256([]byte(b.String()))
	return fmt.Sprintf("%x", h[:8])
}

// QuoteETag wraps an ETag value in double quotes for HTTP headers.
func QuoteETag(etag string) string {
	return `"` + etag + `"`
}

// UnquoteETag strips surrounding double quotes from an ETag value.
func UnquoteETag(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
