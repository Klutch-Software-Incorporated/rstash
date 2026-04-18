package db

import (
	"fmt"
	"strconv"
	"strings"

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
		&model.EgressUsage{},
	); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	// Data migration: delete legacy folder nodes. Folders are now implicit
	// (derived from document paths), so explicit folder rows are no longer needed.
	if gormDB.Migrator().HasColumn(&model.Node{}, "is_folder") {
		gormDB.Exec("DELETE FROM nodes WHERE is_folder = ?", true)
	}

	if err := migrateQuotaSettings(gormDB); err != nil {
		return fmt.Errorf("migrate quota settings: %w", err)
	}

	return nil
}

// migrateQuotaSettings translates legacy quota_mode/egress_mode settings into
// the new representation where User.StorageQuota and User.EgressQuota are the
// authoritative per-user values (0 = unlimited) and a global cap lives in
// total_storage_limit / total_egress_limit.
//
// Behavior by scenario:
//
//   - Legacy keys present in DB: translate their values as-is.
//   - Legacy keys absent AND real users exist: treat as upgrade from a binary
//     that ran on the old hardcoded defaults (quota_mode=total/50GB,
//     egress_mode=user/500GB). Apply those same values so prod behavior is
//     preserved across the upgrade.
//   - Legacy keys absent AND no real users exist: fresh install. Leave
//     defaults at 0 (unlimited / disabled) per the new self-hoster design.
//
// Runs inside a transaction so partial failures don't leave the settings
// table half-migrated. Idempotent: legacy keys are deleted once processed.
func migrateQuotaSettings(gormDB *gorm.DB) error {
	const (
		legacyTotalDefault = int64(50) << 30  // 50 GB
		legacyUserEgress   = int64(500) << 30 // 500 GB
	)
	legacyKeys := []string{"quota_mode", "quota_total", "quota_user", "egress_mode", "egress_quota_user"}

	return gormDB.Transaction(func(tx *gorm.DB) error {
		legacy := make(map[string]string)
		for _, key := range legacyKeys {
			var s model.Setting
			err := tx.Where("key = ?", key).First(&s).Error
			if err == gorm.ErrRecordNotFound {
				continue
			}
			if err != nil {
				return fmt.Errorf("read legacy setting %s: %w", key, err)
			}
			legacy[key] = s.Value
		}

		// Distinguish "fresh install" from "upgrade on hardcoded defaults"
		// by checking whether any real (non-system) users exist.
		var userCount int64
		if err := tx.Model(&model.User{}).Where("id > 0").Count(&userCount).Error; err != nil {
			return fmt.Errorf("count users for migration: %w", err)
		}

		// Fresh install: no legacy settings and no users. Nothing to do.
		if len(legacy) == 0 && userCount == 0 {
			return nil
		}

		// Upgrade with no legacy settings rows: synthesize the old hardcoded
		// defaults so prod behavior is preserved across the upgrade.
		if len(legacy) == 0 {
			legacy["quota_mode"] = "total"
			legacy["quota_total"] = fmt.Sprintf("%d", legacyTotalDefault)
			legacy["egress_mode"] = "user"
			legacy["egress_quota_user"] = fmt.Sprintf("%d", legacyUserEgress)
		}

		// Storage: preserve old effective limits for existing users.
		switch legacy["quota_mode"] {
		case "user":
			if v, ok := legacy["quota_user"]; ok {
				if n, err := parseByteSizeInt(v); err == nil && n > 0 {
					if err := tx.Model(&model.User{}).
						Where("id > 0 AND storage_quota = 0").
						Update("storage_quota", n).Error; err != nil {
						return fmt.Errorf("stamp storage_quota on users: %w", err)
					}
				}
			}
		case "total":
			if v, ok := legacy["quota_total"]; ok {
				if err := upsertSettingTx(tx, "total_storage_limit", v); err != nil {
					return err
				}
			}
		}

		// Egress: same shape.
		if legacy["egress_mode"] == "user" {
			if v, ok := legacy["egress_quota_user"]; ok {
				if n, err := parseByteSizeInt(v); err == nil && n > 0 {
					if err := tx.Model(&model.User{}).
						Where("id > 0 AND egress_quota = 0").
						Update("egress_quota", n).Error; err != nil {
						return fmt.Errorf("stamp egress_quota on users: %w", err)
					}
				}
			}
		}

		// Drop the legacy keys now that they've been translated.
		if err := tx.Where("key IN ?", legacyKeys).Delete(&model.Setting{}).Error; err != nil {
			return fmt.Errorf("delete legacy quota settings: %w", err)
		}
		return nil
	})
}

// upsertSettingTx writes (key, value) to the settings table inside the given
// transaction, leaving an existing row untouched. Used by the quota migration
// to set total_storage_limit / total_egress_limit without clobbering a value
// a prior partial-migration run may have written.
func upsertSettingTx(tx *gorm.DB, key, value string) error {
	var existing model.Setting
	err := tx.Where("key = ?", key).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if err := tx.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
			return fmt.Errorf("insert setting %s: %w", key, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("read setting %s: %w", key, err)
	}
	return nil
}

// parseByteSizeInt parses a legacy byte-size string like "50GB" or "1024".
// Minimal inline implementation; the full parser in internal/config isn't
// importable from the db package due to layering.
func parseByteSizeInt(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, fmt.Errorf("empty size")
	}
	multipliers := []struct {
		suffix string
		factor int64
	}{
		{"TB", 1 << 40}, {"GB", 1 << 30}, {"MB", 1 << 20}, {"KB", 1 << 10}, {"B", 1},
	}
	for _, m := range multipliers {
		if strings.HasSuffix(s, m.suffix) {
			num := strings.TrimSpace(s[:len(s)-len(m.suffix)])
			f, err := strconv.ParseFloat(num, 64)
			if err != nil {
				return 0, err
			}
			return int64(f * float64(m.factor)), nil
		}
	}
	return strconv.ParseInt(s, 10, 64)
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
