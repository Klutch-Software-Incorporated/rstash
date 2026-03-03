package db

import (
	"gorm.io/gorm"
)

// Repository wraps a *gorm.DB and provides all data-access methods.
type Repository struct {
	db      *gorm.DB
	dialect Dialect
}

// NewRepository creates a new Repository from a GORM database connection.
func NewRepository(db *gorm.DB, dialect Dialect) *Repository {
	return &Repository{db: db, dialect: dialect}
}

// GormDB returns the underlying *gorm.DB for direct access when needed.
func (r *Repository) GormDB() *gorm.DB {
	return r.db
}

// Dialect returns the SQL dialect in use.
func (r *Repository) Dialect() Dialect {
	return r.dialect
}

// Transaction executes fn within a database transaction.
// If fn returns an error, the transaction is rolled back.
func (r *Repository) Transaction(fn func(tx *Repository) error) error {
	return r.db.Transaction(func(gormTx *gorm.DB) error {
		txRepo := &Repository{db: gormTx, dialect: r.dialect}
		return fn(txRepo)
	})
}

// WithTx returns a new Repository bound to the given GORM transaction.
func (r *Repository) WithTx(tx *gorm.DB) *Repository {
	return &Repository{db: tx, dialect: r.dialect}
}
