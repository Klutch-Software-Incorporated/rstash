package db

import (
	"context"
	"fmt"
	"time"

	"gosilo/internal/model"

	"gorm.io/gorm"
)

// CreateSession generates a new session token and CSRF token for the given user,
// with a 7-day expiry.
func (r *Repository) CreateSession(ctx context.Context, userID int64, ip string) (*model.Session, error) {
	token, err := RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	csrf, err := RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate csrf token: %w", err)
	}

	now := time.Now().UTC()
	s := model.Session{
		Token:     token,
		UserID:    userID,
		CSRFToken: csrf,
		CreatedAt: now,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		IP:        &ip,
	}
	if err := r.db.WithContext(ctx).Create(&s).Error; err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &s, nil
}

// GetSessionByToken returns the session with the given token, or nil if not found or expired.
func (r *Repository) GetSessionByToken(ctx context.Context, token string) (*model.Session, error) {
	var s model.Session
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).Where("token = ? AND expires_at > ?", token, now).First(&s).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &s, nil
}

// DeleteSession removes a session by token.
func (r *Repository) DeleteSession(ctx context.Context, token string) error {
	if err := r.db.WithContext(ctx).Where("token = ?", token).Delete(&model.Session{}).Error; err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes all expired sessions.
func (r *Repository) DeleteExpiredSessions(ctx context.Context) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Where("expires_at <= ?", now).Delete(&model.Session{}).Error; err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

// DeleteUserSessionsExcept removes all sessions for a user except the given token.
func (r *Repository) DeleteUserSessionsExcept(ctx context.Context, userID int64, exceptToken string) error {
	if err := r.db.WithContext(ctx).Where("user_id = ? AND token != ?", userID, exceptToken).Delete(&model.Session{}).Error; err != nil {
		return fmt.Errorf("delete user sessions except: %w", err)
	}
	return nil
}

// DeleteUserSessions removes all sessions for a given user.
func (r *Repository) DeleteUserSessions(ctx context.Context, userID int64) error {
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.Session{}).Error; err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

// ListUserSessions returns active (non-expired) sessions for a user.
func (r *Repository) ListUserSessions(ctx context.Context, userID int64) ([]*model.Session, error) {
	now := time.Now().UTC()
	var sessions []*model.Session
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND expires_at > ?", userID, now).
		Order("created_at DESC").
		Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("list user sessions: %w", err)
	}
	return sessions, nil
}

// CountUserSessions returns the count of active (non-expired) sessions for a user.
func (r *Repository) CountUserSessions(ctx context.Context, userID int64) (int64, error) {
	now := time.Now().UTC()
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Session{}).
		Where("user_id = ? AND expires_at > ?", userID, now).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count user sessions: %w", err)
	}
	return count, nil
}

// CountActiveSessions returns the count of all non-expired sessions.
func (r *Repository) CountActiveSessions(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Session{}).
		Where("expires_at > ?", now).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count active sessions: %w", err)
	}
	return count, nil
}

// ActiveUserCount returns the count of distinct users with sessions created since `since`.
func (r *Repository) ActiveUserCount(ctx context.Context, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&model.Session{}).
		Where("created_at >= ?", since).
		Distinct("user_id").
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("active user count: %w", err)
	}
	return count, nil
}

// GetRecentUserSessions returns recent sessions for a user, ordered by creation time descending.
func (r *Repository) GetRecentUserSessions(ctx context.Context, userID int64, limit int) ([]*model.Session, error) {
	var sessions []*model.Session
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&sessions).Error
	if err != nil {
		return nil, fmt.Errorf("get recent user sessions: %w", err)
	}
	return sessions, nil
}
