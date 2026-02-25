package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gosilo/internal/model"
)

// CreateRefreshToken generates a new refresh token linked to an access token.
func CreateRefreshToken(ctx context.Context, q Querier, userID int64, clientID string, scopes []string, accessToken string, lifetime time.Duration) (*model.RefreshToken, error) {
	token, err := RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate refresh token: %w", err)
	}

	scopeStr := strings.Join(scopes, " ")

	var query string
	var args []any
	if lifetime > 0 {
		secs := int64(lifetime.Seconds())
		query = `INSERT INTO refresh_tokens (token, user_id, client_id, scopes, access_token, expires_at)
		 VALUES (?, ?, ?, ?, ?, datetime('now', '+' || ? || ' seconds'))
		 RETURNING token, user_id, client_id, scopes, access_token, created_at, expires_at`
		args = []any{token, userID, clientID, scopeStr, accessToken, secs}
	} else {
		query = `INSERT INTO refresh_tokens (token, user_id, client_id, scopes, access_token)
		 VALUES (?, ?, ?, ?, ?)
		 RETURNING token, user_id, client_id, scopes, access_token, created_at, expires_at`
		args = []any{token, userID, clientID, scopeStr, accessToken}
	}

	var t model.RefreshToken
	var scopeOut string
	err = q.QueryRowContext(ctx, query, args...).Scan(&t.Token, &t.UserID, &t.ClientID, &scopeOut, &t.AccessToken, &t.CreatedAt, &t.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("create refresh token: %w", err)
	}
	t.Scopes = strings.Fields(scopeOut)
	return &t, nil
}

// GetRefreshToken returns a non-expired refresh token, or nil if not found or expired.
func GetRefreshToken(ctx context.Context, q Querier, token string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	var scopeStr string
	err := q.QueryRowContext(ctx,
		`SELECT token, user_id, client_id, scopes, access_token, created_at, expires_at
		 FROM refresh_tokens
		 WHERE token = ? AND (expires_at IS NULL OR expires_at > datetime('now'))`,
		token,
	).Scan(&t.Token, &t.UserID, &t.ClientID, &scopeStr, &t.AccessToken, &t.CreatedAt, &t.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	t.Scopes = strings.Fields(scopeStr)
	return &t, nil
}

// GetRefreshTokenUnfiltered returns a refresh token without checking expiry (for revocation).
func GetRefreshTokenUnfiltered(ctx context.Context, q Querier, token string) (*model.RefreshToken, error) {
	var t model.RefreshToken
	var scopeStr string
	err := q.QueryRowContext(ctx,
		`SELECT token, user_id, client_id, scopes, access_token, created_at, expires_at
		 FROM refresh_tokens
		 WHERE token = ?`,
		token,
	).Scan(&t.Token, &t.UserID, &t.ClientID, &scopeStr, &t.AccessToken, &t.CreatedAt, &t.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh token unfiltered: %w", err)
	}
	t.Scopes = strings.Fields(scopeStr)
	return &t, nil
}

// DeleteRefreshToken deletes a refresh token by its token value.
func DeleteRefreshToken(ctx context.Context, q Querier, token string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE token = ?", token)
	if err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}

// DeleteRefreshTokenByAccessToken deletes refresh tokens linked to a given access token.
func DeleteRefreshTokenByAccessToken(ctx context.Context, q Querier, accessToken string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE access_token = ?", accessToken)
	if err != nil {
		return fmt.Errorf("delete refresh token by access token: %w", err)
	}
	return nil
}

// DeleteRefreshTokensByUserID deletes all refresh tokens for a user.
func DeleteRefreshTokensByUserID(ctx context.Context, q Querier, userID int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE user_id = ?", userID)
	if err != nil {
		return fmt.Errorf("delete refresh tokens by user id: %w", err)
	}
	return nil
}

// DeleteExpiredRefreshTokens removes all expired refresh tokens.
func DeleteExpiredRefreshTokens(ctx context.Context, q Querier) error {
	_, err := q.ExecContext(ctx, "DELETE FROM refresh_tokens WHERE expires_at IS NOT NULL AND expires_at <= datetime('now')")
	if err != nil {
		return fmt.Errorf("delete expired refresh tokens: %w", err)
	}
	return nil
}
