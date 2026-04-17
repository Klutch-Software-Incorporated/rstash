package blob

import (
	"fmt"
	"slices"

	"rstash/internal/db"
)

// SupportedSchemes is the canonical list of DSN schemes the blob package
// knows how to open. It's the source of truth consulted by both openInner
// (below) and internal/config.Validate — the two can't drift out of sync
// without breaking compilation.
var SupportedSchemes = []string{"sqlite", "fs", "postgres", "mysql", "mssql", "s3", "azureblob"}

// IsSupportedScheme returns true if scheme is one rstash can open.
func IsSupportedScheme(scheme string) bool {
	return slices.Contains(SupportedSchemes, scheme)
}

// OpenStore opens a blob store from a DSN scheme and path.
//
// When RSTASH_BLOB_KEY is set (base64 of 32 bytes), the returned store is
// wrapped with AES-256-GCM encryption. Writes encrypt; reads auto-detect
// via a magic prefix so pre-encryption blobs continue to work.
func OpenStore(scheme, path, rawDSN string) (Store, error) {
	inner, err := openInner(scheme, path, rawDSN)
	if err != nil {
		return nil, err
	}
	return Wrap(inner, EnvKeyProvider{}), nil
}

func openInner(scheme, path, rawDSN string) (Store, error) {
	switch scheme {
	case "fs":
		return NewFSStore(path)
	case "sqlite":
		return NewSQLiteStore(path)
	case "postgres", "mysql", "mssql":
		gormDB, _, err := db.OpenGORM(rawDSN)
		if err != nil {
			return nil, fmt.Errorf("open blob database: %w", err)
		}
		return NewGORMStore(gormDB)
	case "s3":
		return NewS3Store(path)
	case "azureblob":
		return NewAzureBlobStore(path)
	default:
		return nil, fmt.Errorf("unsupported blob scheme %q", scheme)
	}
}
