package db

import (
	"fmt"

	"rstash/internal/model"

	"gorm.io/gorm"
)

// Migrate runs GORM AutoMigrate for all model types and applies data migrations.
func Migrate(gormDB *gorm.DB) error {
	if err := gormDB.AutoMigrate(
		&model.User{},
		&model.OAuthClient{},
		&model.OAuthToken{},
		&model.Node{},
		&model.Session{},
		&model.AuditEntry{},
		&model.AuthorizationCode{},
		&model.Setting{},
		&model.RefreshToken{},
		&model.AbuseReport{},
		&model.EmailSend{},
		&model.APIKey{},
		&model.WebhookSubscription{},
		&model.WebhookDelivery{},
		&model.BandwidthUsage{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	// Data migration: delete legacy folder nodes. Folders are now implicit
	// (derived from document paths), so explicit folder rows are no longer needed.
	if gormDB.Migrator().HasColumn(&model.Node{}, "is_folder") {
		gormDB.Exec("DELETE FROM nodes WHERE is_folder = ?", true)
	}

	return nil
}

// EnsureSystemUser creates the _system sentinel user (ID 0) if it does not exist.
func EnsureSystemUser(gormDB *gorm.DB, dialect Dialect) error {
	var count int64
	if err := gormDB.Model(&model.User{}).Where("id = ?", 0).Count(&count).Error; err != nil {
		return fmt.Errorf("check system user: %w", err)
	}
	if count == 0 {
		// Use a transaction to ensure all statements run on the same connection.
		// This is critical for MSSQL (IDENTITY_INSERT is connection-scoped) and
		// MySQL (session sql_mode is connection-scoped).
		err := gormDB.Transaction(func(tx *gorm.DB) error {
			switch dialect {
			case DialectMySQL:
				// MySQL treats INSERT of 0 into AUTO_INCREMENT as "next value" by default.
				if err := tx.Exec("SET SESSION sql_mode = CONCAT(@@sql_mode, ',NO_AUTO_VALUE_ON_ZERO')").Error; err != nil {
					return fmt.Errorf("set NO_AUTO_VALUE_ON_ZERO: %w", err)
				}
			case DialectSQLServer:
				// SQL Server requires IDENTITY_INSERT to insert explicit identity values.
				if err := tx.Exec("SET IDENTITY_INSERT users ON").Error; err != nil {
					return fmt.Errorf("set IDENTITY_INSERT ON: %w", err)
				}
			}

			// Use raw SQL to insert with explicit id=0, since GORM's Create
			// may skip zero-value primary keys on some dialects.
			if err := tx.Exec(
				"INSERT INTO users (id, username, password_hash, is_admin, disabled, approved, created_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)",
				0, "_system", "", false, true, true,
			).Error; err != nil {
				return fmt.Errorf("insert system user: %w", err)
			}

			if dialect == DialectSQLServer {
				if err := tx.Exec("SET IDENTITY_INSERT users OFF").Error; err != nil {
					return fmt.Errorf("set IDENTITY_INSERT OFF: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("create system user: %w", err)
		}
	}
	return nil
}
