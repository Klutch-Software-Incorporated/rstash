package db

import (
	"fmt"
)

// OpenRepository opens a GORM database, runs migrations, ensures the system
// user exists, and returns a ready-to-use Repository.
func OpenRepository(dsn string) (*Repository, error) {
	gormDB, dialect, err := OpenGORM(dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := Migrate(gormDB); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := EnsureSystemUser(gormDB, dialect); err != nil {
		return nil, fmt.Errorf("ensure system user: %w", err)
	}

	return NewRepository(gormDB, dialect), nil
}

// Ping verifies the database connection is alive.
func (r *Repository) Ping() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

// Close closes the underlying database connection.
func (r *Repository) Close() error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
