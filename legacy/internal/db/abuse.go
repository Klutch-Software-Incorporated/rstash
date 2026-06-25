package db

import (
	"context"
	"fmt"
	"time"

	"rstash/internal/model"

	"gorm.io/gorm"
)

// CreateAbuseReport inserts a new abuse report.
func (r *Repository) CreateAbuseReport(ctx context.Context, email, path, reason, description, ip string) (*model.AbuseReport, error) {
	report := model.AbuseReport{
		ReporterEmail: email,
		ReportedPath:  path,
		Reason:        reason,
		Description:   &description,
		ReporterIP:    &ip,
	}
	if err := r.db.WithContext(ctx).Create(&report).Error; err != nil {
		return nil, fmt.Errorf("create abuse report: %w", err)
	}
	return &report, nil
}

// AbuseReportRow is an extended abuse report with the reviewer's username.
type AbuseReportRow struct {
	model.AbuseReport
	ReviewerUsername string
}

// ListAbuseReports returns abuse reports filtered by status, with pagination.
func (r *Repository) ListAbuseReports(ctx context.Context, status string, limit, offset int) ([]*AbuseReportRow, error) {
	var reports []*AbuseReportRow
	q := r.db.WithContext(ctx).
		Table("abuse_reports r").
		Select("r.id, r.reporter_email, r.reported_path, r.reason, r.description, r.status, r.reporter_ip, r.reviewer_id, r.review_note, r.created_at, r.reviewed_at, COALESCE(u.username, '') AS reviewer_username").
		Joins("LEFT JOIN users u ON r.reviewer_id = u.id").
		Order("r.created_at DESC").
		Limit(limit).
		Offset(offset)
	if status != "" {
		q = q.Where("r.status = ?", status)
	}
	if err := q.Scan(&reports).Error; err != nil {
		return nil, fmt.Errorf("list abuse reports: %w", err)
	}
	return reports, nil
}

// GetAbuseReport returns a single abuse report by ID.
func (r *Repository) GetAbuseReport(ctx context.Context, id int64) (*model.AbuseReport, error) {
	var report model.AbuseReport
	err := r.db.WithContext(ctx).First(&report, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get abuse report: %w", err)
	}
	return &report, nil
}

// UpdateAbuseReportStatus updates the status of an abuse report with reviewer info.
func (r *Repository) UpdateAbuseReportStatus(ctx context.Context, id int64, status string, reviewerID int64, note string) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Model(&model.AbuseReport{}).Where("id = ?", id).Updates(map[string]any{
		"status":      status,
		"reviewer_id": reviewerID,
		"review_note": note,
		"reviewed_at": now,
	}).Error; err != nil {
		return fmt.Errorf("update abuse report status: %w", err)
	}
	return nil
}

// CountOpenAbuseReports returns the number of abuse reports with status "open".
func (r *Repository) CountOpenAbuseReports(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.AbuseReport{}).Where("status = ?", "open").Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count open abuse reports: %w", err)
	}
	return count, nil
}
