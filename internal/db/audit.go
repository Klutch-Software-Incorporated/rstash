package db

import (
	"context"
	"fmt"

	"gosilo/internal/model"
)

// InsertAuditEntry records an audit log entry.
func InsertAuditEntry(ctx context.Context, q Querier, actorID int64, action, targetType, targetID, details string) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO audit_log (actor_id, action, target_type, target_id, details)
		 VALUES (?, ?, ?, ?, ?)`,
		actorID, action, targetType, targetID, details,
	)
	if err != nil {
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
func ListAuditEntries(ctx context.Context, q Querier, limit, offset int) ([]*AuditRow, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT a.id, a.actor_id, u.username, a.action, a.target_type, a.target_id,
		        COALESCE(a.details, ''), a.created_at
		 FROM audit_log a
		 JOIN users u ON u.id = a.actor_id
		 ORDER BY a.created_at DESC
		 LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()

	var entries []*AuditRow
	for rows.Next() {
		var e AuditRow
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorUsername, &e.Action,
			&e.TargetType, &e.TargetID, &e.Details, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// ListAuditEntriesByTarget returns recent audit entries for a specific target.
func ListAuditEntriesByTarget(ctx context.Context, q Querier, targetType, targetID string, limit int) ([]*AuditRow, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT a.id, a.actor_id, u.username, a.action, a.target_type, a.target_id,
		        COALESCE(a.details, ''), a.created_at
		 FROM audit_log a
		 JOIN users u ON u.id = a.actor_id
		 WHERE a.target_type = ? AND a.target_id = ?
		 ORDER BY a.created_at DESC
		 LIMIT ?`,
		targetType, targetID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list audit entries by target: %w", err)
	}
	defer rows.Close()

	var entries []*AuditRow
	for rows.Next() {
		var e AuditRow
		if err := rows.Scan(&e.ID, &e.ActorID, &e.ActorUsername, &e.Action,
			&e.TargetType, &e.TargetID, &e.Details, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

// CountAuditEntries returns the total number of audit log entries.
func CountAuditEntries(ctx context.Context, q Querier) (int64, error) {
	var count int64
	err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM audit_log").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count audit entries: %w", err)
	}
	return count, nil
}
