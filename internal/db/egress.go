package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"rstash/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CurrentPeriod returns the period string ("YYYY-MM") for the current UTC time.
func CurrentPeriod() string {
	return time.Now().UTC().Format("2006-01")
}

// GetEgressUsage returns the usage row for (userID, period), or nil when
// the user has no egress recorded yet for the period.
func (r *Repository) GetEgressUsage(ctx context.Context, userID int64, period string) (*model.EgressUsage, error) {
	var u model.EgressUsage
	err := r.db.WithContext(ctx).Where("user_id = ? AND period = ?", userID, period).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get egress usage: %w", err)
	}
	return &u, nil
}

// AddEgressUsage atomically increments the (userID, period) row by bytes.
// Creates the row if it doesn't exist. Safe for high-frequency calls on the
// storage read path; the storage service batches internally to further reduce
// write pressure.
func (r *Repository) AddEgressUsage(ctx context.Context, userID int64, period string, bytes int64) error {
	if bytes <= 0 {
		return nil
	}
	usage := model.EgressUsage{
		UserID:    userID,
		Period:    period,
		BytesOut:  bytes,
		UpdatedAt: time.Now().UTC(),
	}
	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "period"}},
		DoUpdates: clause.Assignments(map[string]any{
			"bytes_out":  gorm.Expr("bytes_out + ?", bytes),
			"updated_at": time.Now().UTC(),
		}),
	}).Create(&usage).Error
	if err != nil {
		return fmt.Errorf("add egress usage: %w", err)
	}
	return nil
}

// ResetEgressUsage zeros the current-period counter for userID. Used by the
// admin API to align with external billing periods or resolve disputes.
func (r *Repository) ResetEgressUsage(ctx context.Context, userID int64, period string) error {
	err := r.db.WithContext(ctx).Model(&model.EgressUsage{}).
		Where("user_id = ? AND period = ?", userID, period).
		Updates(map[string]any{"bytes_out": 0, "updated_at": time.Now().UTC()}).Error
	if err != nil {
		return fmt.Errorf("reset egress usage: %w", err)
	}
	return nil
}

// UpdateUserEgressQuota sets the per-user monthly egress override (0 = use default).
func (r *Repository) UpdateUserEgressQuota(ctx context.Context, userID int64, quotaBytes int64) error {
	if err := r.db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).
		Update("egress_quota", quotaBytes).Error; err != nil {
		return fmt.Errorf("update user egress quota: %w", err)
	}
	return nil
}
