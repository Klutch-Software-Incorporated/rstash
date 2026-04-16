package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"rstash/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UpsertOAuthClient inserts or updates an OAuth client by ID.
func (r *Repository) UpsertOAuthClient(ctx context.Context, id, redirectURI string) (*model.OAuthClient, error) {
	c := model.OAuthClient{
		ID:          id,
		RedirectURI: redirectURI,
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"redirect_uri"}),
	}).Create(&c)
	if result.Error != nil {
		return nil, fmt.Errorf("upsert oauth client: %w", result.Error)
	}
	// Re-read for accurate timestamps.
	var out model.OAuthClient
	if err := r.db.WithContext(ctx).First(&out, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("upsert oauth client read-back: %w", err)
	}
	return &out, nil
}

// GetOAuthClient returns an OAuth client by ID, or nil if not found.
func (r *Repository) GetOAuthClient(ctx context.Context, id string) (*model.OAuthClient, error) {
	var c model.OAuthClient
	err := r.db.WithContext(ctx).First(&c, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get oauth client: %w", err)
	}
	return &c, nil
}

// CreateOAuthToken generates a new OAuth access token.
func (r *Repository) CreateOAuthToken(ctx context.Context, userID int64, clientID string, scopes []string, lifetime time.Duration) (*model.OAuthToken, error) {
	token, err := RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate oauth token: %w", err)
	}

	now := time.Now().UTC()
	t := model.OAuthToken{
		Token:     token,
		UserID:    userID,
		ClientID:  clientID,
		Scopes:    strings.Join(scopes, " "),
		CreatedAt: now,
	}
	if lifetime > 0 {
		exp := now.Add(lifetime)
		t.ExpiresAt = &exp
	}

	if err := r.db.WithContext(ctx).Create(&t).Error; err != nil {
		return nil, fmt.Errorf("create oauth token: %w", err)
	}
	return &t, nil
}

// GetOAuthToken returns the token record, or nil if not found or expired.
func (r *Repository) GetOAuthToken(ctx context.Context, token string) (*model.OAuthToken, error) {
	var t model.OAuthToken
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).
		Where("token = ? AND (expires_at IS NULL OR expires_at > ?)", token, now).
		First(&t).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get oauth token: %w", err)
	}
	return &t, nil
}

// GetOAuthTokenUnfiltered returns the token record without checking expiry (for revocation).
func (r *Repository) GetOAuthTokenUnfiltered(ctx context.Context, token string) (*model.OAuthToken, error) {
	var t model.OAuthToken
	err := r.db.WithContext(ctx).First(&t, "token = ?", token).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get oauth token unfiltered: %w", err)
	}
	return &t, nil
}

// ListOAuthTokensByUserID returns all non-expired tokens for the given user.
func (r *Repository) ListOAuthTokensByUserID(ctx context.Context, userID int64) ([]*model.OAuthToken, error) {
	now := time.Now().UTC()
	var tokens []*model.OAuthToken
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, now).
		Order("created_at DESC").
		Find(&tokens).Error
	if err != nil {
		return nil, fmt.Errorf("list oauth tokens by user: %w", err)
	}
	return tokens, nil
}

// DeleteOAuthToken revokes a token.
func (r *Repository) DeleteOAuthToken(ctx context.Context, token string) error {
	if err := r.db.WithContext(ctx).Where("token = ?", token).Delete(&model.OAuthToken{}).Error; err != nil {
		return fmt.Errorf("delete oauth token: %w", err)
	}
	return nil
}

// DeleteOAuthTokensByUserID deletes all OAuth tokens for a user.
func (r *Repository) DeleteOAuthTokensByUserID(ctx context.Context, userID int64) error {
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.OAuthToken{}).Error; err != nil {
		return fmt.Errorf("delete oauth tokens by user id: %w", err)
	}
	return nil
}

// DeleteUserOAuthTokensForClient deletes all OAuth (and refresh) tokens the
// user has issued to a specific client. Returns the number of access tokens removed.
func (r *Repository) DeleteUserOAuthTokensForClient(ctx context.Context, userID int64, clientID string) (int64, error) {
	res := r.db.WithContext(ctx).Where("user_id = ? AND client_id = ?", userID, clientID).
		Delete(&model.OAuthToken{})
	if res.Error != nil {
		return 0, fmt.Errorf("delete oauth tokens: %w", res.Error)
	}
	if err := r.db.WithContext(ctx).Where("user_id = ? AND client_id = ?", userID, clientID).
		Delete(&model.RefreshToken{}).Error; err != nil {
		return 0, fmt.Errorf("delete refresh tokens: %w", err)
	}
	return res.RowsAffected, nil
}

// DeleteExpiredOAuthTokens removes all expired tokens.
func (r *Repository) DeleteExpiredOAuthTokens(ctx context.Context) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Where("expires_at IS NOT NULL AND expires_at <= ?", now).Delete(&model.OAuthToken{}).Error; err != nil {
		return fmt.Errorf("delete expired oauth tokens: %w", err)
	}
	return nil
}

// CountUserOAuthTokens returns the count of active (non-expired) OAuth tokens for a user.
func (r *Repository) CountUserOAuthTokens(ctx context.Context, userID int64) (int64, error) {
	now := time.Now().UTC()
	var count int64
	err := r.db.WithContext(ctx).Model(&model.OAuthToken{}).
		Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userID, now).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count user oauth tokens: %w", err)
	}
	return count, nil
}

// CountActiveOAuthTokens returns the count of all non-expired OAuth tokens.
func (r *Repository) CountActiveOAuthTokens(ctx context.Context) (int64, error) {
	now := time.Now().UTC()
	var count int64
	err := r.db.WithContext(ctx).Model(&model.OAuthToken{}).
		Where("expires_at IS NULL OR expires_at > ?", now).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count active oauth tokens: %w", err)
	}
	return count, nil
}

// GetRecentUserOAuthTokens returns recently created OAuth tokens for a user.
func (r *Repository) GetRecentUserOAuthTokens(ctx context.Context, userID int64, limit int) ([]*model.OAuthToken, error) {
	var tokens []*model.OAuthToken
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&tokens).Error
	if err != nil {
		return nil, fmt.Errorf("get recent user oauth tokens: %w", err)
	}
	return tokens, nil
}

// CreateAuthorizationCode generates a new authorization code for the OAuth code flow.
func (r *Repository) CreateAuthorizationCode(ctx context.Context, userID int64, clientID, redirectURI, scopes, codeChallenge, codeChallengeMethod string) (*model.AuthorizationCode, error) {
	code, err := RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate authorization code: %w", err)
	}

	now := time.Now().UTC()
	ac := model.AuthorizationCode{
		Code:                code,
		UserID:              userID,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scopes:              scopes,
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		CreatedAt:           now,
		ExpiresAt:           now.Add(10 * time.Minute),
		Used:                false,
	}
	if err := r.db.WithContext(ctx).Create(&ac).Error; err != nil {
		return nil, fmt.Errorf("create authorization code: %w", err)
	}
	return &ac, nil
}

// GetAuthorizationCode returns the authorization code record, or nil if not found, expired, or already used.
func (r *Repository) GetAuthorizationCode(ctx context.Context, code string) (*model.AuthorizationCode, error) {
	var ac model.AuthorizationCode
	now := time.Now().UTC()
	err := r.db.WithContext(ctx).
		Where("code = ? AND used = ? AND expires_at > ?", code, false, now).
		First(&ac).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get authorization code: %w", err)
	}
	return &ac, nil
}

// UseAuthorizationCode marks an authorization code as used.
func (r *Repository) UseAuthorizationCode(ctx context.Context, code string) error {
	if err := r.db.WithContext(ctx).Model(&model.AuthorizationCode{}).Where("code = ?", code).Update("used", true).Error; err != nil {
		return fmt.Errorf("use authorization code: %w", err)
	}
	return nil
}

// DeleteExpiredAuthorizationCodes removes expired or used authorization codes.
func (r *Repository) DeleteExpiredAuthorizationCodes(ctx context.Context) error {
	now := time.Now().UTC()
	if err := r.db.WithContext(ctx).Where("used = ? OR expires_at <= ?", true, now).Delete(&model.AuthorizationCode{}).Error; err != nil {
		return fmt.Errorf("delete expired authorization codes: %w", err)
	}
	return nil
}
