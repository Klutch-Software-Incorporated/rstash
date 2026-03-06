package db

import (
	"context"
	"fmt"
	"time"

	"rstash/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetSetting returns the value for the given key, or "" if not found.
func (r *Repository) GetSetting(ctx context.Context, key string) (string, error) {
	var s model.Setting
	err := r.db.WithContext(ctx).First(&s, "key = ?", key).Error
	if err == gorm.ErrRecordNotFound {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get setting %q: %w", key, err)
	}
	return s.Value, nil
}

// SetSetting upserts a setting key-value pair.
func (r *Repository) SetSetting(ctx context.Context, key, value string) error {
	now := time.Now().UTC()
	s := model.Setting{
		Key:       key,
		Value:     value,
		UpdatedAt: now,
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&s)
	if result.Error != nil {
		return fmt.Errorf("set setting %q: %w", key, result.Error)
	}
	return nil
}

// DeleteSetting removes a setting by key.
func (r *Repository) DeleteSetting(ctx context.Context, key string) error {
	if err := r.db.WithContext(ctx).Where("key = ?", key).Delete(&model.Setting{}).Error; err != nil {
		return fmt.Errorf("delete setting %q: %w", key, err)
	}
	return nil
}

// ListSettings returns all settings as a map.
func (r *Repository) ListSettings(ctx context.Context) (map[string]string, error) {
	var settings []model.Setting
	if err := r.db.WithContext(ctx).Find(&settings).Error; err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	result := make(map[string]string, len(settings))
	for _, s := range settings {
		result[s.Key] = s.Value
	}
	return result, nil
}
