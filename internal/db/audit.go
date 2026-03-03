package db

import (
	"context"
	"fmt"
	"log/slog"

	"gosilo/internal/model"
)

// SystemActorID is the user ID of the _system sentinel user, used as the
// actor for CLI operations and system events.
const SystemActorID int64 = 0

// Audit is a convenience wrapper that logs an audit entry and swallows errors.
func (r *Repository) Audit(ctx context.Context, actorID int64, action, targetType, targetID, details string) {
	if err := r.InsertAuditEntry(ctx, actorID, action, targetType, targetID, details); err != nil {
		slog.Error("failed to write audit log", "error", err, "action", action)
	}
}

// InsertAuditEntry records an audit log entry.
func (r *Repository) InsertAuditEntry(ctx context.Context, actorID int64, action, targetType, targetID, details string) error {
	entry := model.AuditEntry{
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Details:    details,
	}
	if err := r.db.WithContext(ctx).Create(&entry).Error; err != nil {
		return fmt.Errorf("insert audit entry: %w", err)
	}
	return nil
}

// AuditRow is a joined audit entry with the actor's username.
type AuditRow struct {
	model.AuditEntry
	ActorUsername string
}

// ListAuditEntries returns recent audit entries joined with actor username.
func (r *Repository) ListAuditEntries(ctx context.Context, limit, offset int) ([]*AuditRow, error) {
	var entries []*AuditRow
	q := r.db.WithContext(ctx).
		Table("audit_log a").
		Select("a.id, a.actor_id, COALESCE(u.username, '_system') AS actor_username, a.action, a.target_type, a.target_id, COALESCE(a.details, '') AS details, a.created_at").
		Joins("LEFT JOIN users u ON u.id = a.actor_id").
		Order("a.created_at DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	if offset > 0 {
		q = q.Offset(offset)
	}
	if err := q.Scan(&entries).Error; err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	return entries, nil
}

// ListAuditEntriesByTarget returns recent audit entries for a specific target.
func (r *Repository) ListAuditEntriesByTarget(ctx context.Context, targetType, targetID string, limit int) ([]*AuditRow, error) {
	var entries []*AuditRow
	err := r.db.WithContext(ctx).
		Table("audit_log a").
		Select("a.id, a.actor_id, COALESCE(u.username, '_system') AS actor_username, a.action, a.target_type, a.target_id, COALESCE(a.details, '') AS details, a.created_at").
		Joins("LEFT JOIN users u ON u.id = a.actor_id").
		Where("a.target_type = ? AND a.target_id = ?", targetType, targetID).
		Order("a.created_at DESC").
		Limit(limit).
		Scan(&entries).Error
	if err != nil {
		return nil, fmt.Errorf("list audit entries by target: %w", err)
	}
	return entries, nil
}

// CountAuditEntries returns the total number of audit log entries.
func (r *Repository) CountAuditEntries(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AuditEntry{}).Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count audit entries: %w", err)
	}
	return count, nil
}
