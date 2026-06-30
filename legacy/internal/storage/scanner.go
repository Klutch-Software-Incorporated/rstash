package storage

import "context"

// ScanResult holds the outcome of a content scan.
type ScanResult struct {
	Allowed bool
	Reason  string
}

// ContentScanner inspects uploaded content and decides whether to allow it.
type ContentScanner interface {
	Scan(ctx context.Context, data []byte, contentType string, userID int64, path string) ScanResult
}
