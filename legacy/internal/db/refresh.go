package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"rstash/internal/model"

	"gorm.io/gorm"
)

// CreateRefreshToken generates a new refresh token linked to an access token.
func (r *Repository) CreateRefreshToken(ctx context.Context, userID int64, clientID string, scopes []string, accessToken string, lifetime time.Duration) (*model.RefreshToken, error) {
	token, err := RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	now := time.Now().UTC()
	t := model.RefreshToken{
		Token:       token,
		UserID:      userID,
		ClientID:    clientID,
		Scopes:      strings.Join(scopes, " "),
		AccessToken: accessToken,
		CreatedAt:   now,
	}
	if lifetime > 0 {
		exp := now.Add(lifetime)
		t.ExpiresAt = &exp
	}

	if err := r.db.WithContext(ctx).Create(&t).Error; err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	return &t, nil
}

// GetRefreshToken returns a non-expired refresh token, or nil if not found or expired.
func (r *Repository) GetRefreshToken(ctx context.Context, token string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).
		Where("token = ? AND (expires_at IS NULL OR expires_at > ?)", token, now).
		First(&t).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	return &t, nil
}

// GetRefreshTokenUnfiltered returns a refresh token without checking expiry (for revocation).
func (r *Repository) GetRefreshTokenUnfiltered(ctx context.Context, token string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	err := r.db.WithContext(ctx).First(&t, "token = ?", token).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh token unfiltered: %w", err)
	}
	return &t, nil
}

// DeleteRefreshToken deletes a refresh token by its token value.
func (r *Repository) DeleteRefreshToken(ctx context.Context, token string) error {
	if err := r.db.WithContext(ctx).Where("token = ?", token).Delete(&model.RefreshToken{}).Error; err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}

// DeleteRefreshTokenByAccessToken deletes refresh tokens linked to a given access token.
func (r *Repository) DeleteRefreshTokenByAccessToken(ctx context.Context, accessToken string) error {
	if err := r.db.WithContext(ctx).Where("access_token = ?", accessToken).Delete(&model.RefreshToken{}).Error; err != nil {
		return fmt.Errorf("delete refresh token by access token: %w", err)
	}
	return nil
}

// DeleteRefreshTokensByUserID deletes all refresh tokens for a user.
func (r *Repository) DeleteRefreshTokensByUserID(ctx context.Context, userID int64) error {
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.RefreshToken{}).Error; err != nil {
		return fmt.Errorf("delete refresh tokens by user id: %w", err)
	}
	return nil
}

// DeleteExpiredRefreshTokens removes all expired refresh tokens.
func (r *Repository) DeleteExpiredRefreshTokens(ctx context.Context) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Where("expires_at IS NOT NULL AND expires_at <= ?", now).Delete(&model.RefreshToken{}).Error; err != nil {
		return fmt.Errorf("delete expired refresh tokens: %w", err)
	}
	return nil
}
