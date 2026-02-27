package db

import (
	"context"
	"database/sql"
	"fmt"

	"gosilo/internal/model"
)

// CreateAbuseReport inserts a new abuse report.
func CreateAbuseReport(ctx context.Context, q Querier, email, path, reason, description, ip string) (*model.AbuseReport, error) {
	var r model.AbuseReport
	err := q.QueryRowContext(ctx,
		`INSERT INTO abuse_reports (reporter_email, reported_path, reason, description, reporter_ip)
		 VALUES (?, ?, ?, ?, ?)
		 RETURNING id, reporter_email, reported_path, reason, description, status, reporter_ip, reviewer_id, review_note, created_at, reviewed_at`,
		email, path, reason, description, ip,
	).Scan(&r.ID, &r.ReporterEmail, &r.ReportedPath, &r.Reason, &r.Description, &r.Status, &r.ReporterIP, &r.ReviewerID, &r.ReviewNote, &r.CreatedAt, &r.ReviewedAt)
	if err != nil {
		return nil, fmt.Errorf("create abuse report: %w", err)
	}
	return &r, nil
}

// AbuseReportRow is an extended abuse report with the reviewer's username.
type AbuseReportRow struct {
	model.AbuseReport
	ReviewerUsername string
}

// ListAbuseReports returns abuse reports filtered by status, with pagination.
// Pass status="" to list all.
func ListAbuseReports(ctx context.Context, q Querier, status string, limit, offset int) ([]*AbuseReportRow, error) {
	var query string
	var args []any

	if status != "" {
		query = `SELECT r.id, r.reporter_email, r.reported_path, r.reason, r.description,
		         r.status, r.reporter_ip, r.reviewer_id, r.review_note, r.created_at, r.reviewed_at,
		         COALESCE(u.username, '')
		         FROM abuse_reports r LEFT JOIN users u ON r.reviewer_id = u.id
		         WHERE r.status = ? ORDER BY r.created_at DESC LIMIT ? OFFSET ?`
		args = []any{status, limit, offset}
	} else {
		query = `SELECT r.id, r.reporter_email, r.reported_path, r.reason, r.description,
		         r.status, r.reporter_ip, r.reviewer_id, r.review_note, r.created_at, r.reviewed_at,
		         COALESCE(u.username, '')
		         FROM abuse_reports r LEFT JOIN users u ON r.reviewer_id = u.id
		         ORDER BY r.created_at DESC LIMIT ? OFFSET ?`
		args = []any{limit, offset}
	}

	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list abuse reports: %w", err)
	}
	defer rows.Close()

	var reports []*AbuseReportRow
	for rows.Next() {
		var r AbuseReportRow
		if err := rows.Scan(&r.ID, &r.ReporterEmail, &r.ReportedPath, &r.Reason, &r.Description,
			&r.Status, &r.ReporterIP, &r.ReviewerID, &r.ReviewNote, &r.CreatedAt, &r.ReviewedAt,
			&r.ReviewerUsername); err != nil {
			return nil, fmt.Errorf("scan abuse report: %w", err)
		}
		reports = append(reports, &r)
	}
	return reports, rows.Err()
}

// GetAbuseReport returns a single abuse report by ID.
func GetAbuseReport(ctx context.Context, q Querier, id int64) (*model.AbuseReport, error) {
	var r model.AbuseReport
	err := q.QueryRowContext(ctx,
		`SELECT id, reporter_email, reported_path, reason, description, status, reporter_ip, reviewer_id, review_note, created_at, reviewed_at
		 FROM abuse_reports WHERE id = ?`, id,
	).Scan(&r.ID, &r.ReporterEmail, &r.ReportedPath, &r.Reason, &r.Description, &r.Status, &r.ReporterIP, &r.ReviewerID, &r.ReviewNote, &r.CreatedAt, &r.ReviewedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get abuse report: %w", err)
	}
	return &r, nil
}

// UpdateAbuseReportStatus updates the status of an abuse report with reviewer info.
func UpdateAbuseReportStatus(ctx context.Context, q Querier, id int64, status string, reviewerID int64, note string) error {
	_, err := q.ExecContext(ctx,
		`UPDATE abuse_reports SET status = ?, reviewer_id = ?, review_note = ?, reviewed_at = datetime('now') WHERE id = ?`,
		status, reviewerID, note, id,
	)
	if err != nil {
		return fmt.Errorf("update abuse report status: %w", err)
	}
	return nil
}

// CountOpenAbuseReports returns the number of abuse reports with status "open".
func CountOpenAbuseReports(ctx context.Context, q Querier) (int64, error) {
	var count int64
	err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM abuse_reports WHERE status = 'open'").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count open abuse reports: %w", err)
	}
	return count, nil
}
