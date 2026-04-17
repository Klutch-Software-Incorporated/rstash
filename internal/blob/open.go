package blob

import (
	"fmt"

	"rstash/internal/db"
)

// OpenStore opens a blob store from a DSN scheme and path.
// Supported schemes: sqlite, fs, postgres, mysql, mssql, s3.
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
	default:
		return nil, fmt.Errorf("unsupported blob scheme %q", scheme)
	}
}
