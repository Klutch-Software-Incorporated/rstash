package db

import (
	"context"
	"time"

	"rstash/internal/model"
)

// LogEmailSend records an email send attempt.
func (r *Repository) LogEmailSend(ctx context.Context, to, typ, subject, status, errMsg string) error {
	entry := &model.EmailSend{
		To:        to,
		Type:      typ,
		Subject:   subject,
		Status:    status,
		Error:     errMsg,
		CreatedAt: time.Now().UTC(),
	}
	return r.db.WithContext(ctx).Create(entry).Error
}

// CountEmailSends returns the number of email sends since the given time.
func (r *Repository) CountEmailSends(ctx context.Context, since time.Time) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.EmailSend{}).Where("created_at >= ?", since).Count(&count).Error
	return int(count), err
}

// CountUsersWithEmail returns the number of non-system users with a non-null email.
func (r *Repository) CountUsersWithEmail(ctx context.Context) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.User{}).Where("id >= ? AND email IS NOT NULL", 1).Count(&count).Error
	return int(count), err
}
