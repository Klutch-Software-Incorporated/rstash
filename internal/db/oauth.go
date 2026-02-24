package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"

	"gosilo/internal/model"
)

// UpsertOAuthClient inserts or updates an OAuth client by ID.
func UpsertOAuthClient(ctx context.Context, q Querier, id, redirectURI string) (*model.OAuthClient, error) {
	var c model.OAuthClient
	err := q.QueryRowContext(ctx,
		`INSERT INTO oauth_clients (id, redirect_uri)
		 VALUES (?, ?)
		 ON CONFLICT(id) DO UPDATE SET redirect_uri = excluded.redirect_uri
		 RETURNING id, redirect_uri, created_at`,
		id, redirectURI,
	).Scan(&c.ID, &c.RedirectURI, &c.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert oauth client: %w", err)
	}
	return &c, nil
}

// GetOAuthClient returns an OAuth client by ID, or nil if not found.
func GetOAuthClient(ctx context.Context, q Querier, id string) (*model.OAuthClient, error) {
	var c model.OAuthClient
	err := q.QueryRowContext(ctx,
		`SELECT id, redirect_uri, created_at FROM oauth_clients WHERE id = ?`,
		id,
	).Scan(&c.ID, &c.RedirectURI, &c.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get oauth client: %w", err)
	}
	return &c, nil
}

// CreateOAuthToken generates a new OAuth access token for the given user and client.
// Scopes are stored as a space-separated string. No expiry (revoke-only).
func CreateOAuthToken(ctx context.Context, q Querier, userID int64, clientID string, scopes []string) (*model.OAuthToken, error) {
	token, err := oauthRandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate oauth token: %w", err)
	}

	scopeStr := strings.Join(scopes, " ")

	var t model.OAuthToken
	var scopeOut string
	err = q.QueryRowContext(ctx,
		`INSERT INTO oauth_tokens (token, user_id, client_id, scopes)
		 VALUES (?, ?, ?, ?)
		 RETURNING token, user_id, client_id, scopes, created_at, expires_at`,
		token, userID, clientID, scopeStr,
	).Scan(&t.Token, &t.UserID, &t.ClientID, &scopeOut, &t.CreatedAt, &t.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("create oauth token: %w", err)
	}
	t.Scopes = strings.Fields(scopeOut)
	return &t, nil
}

// GetOAuthToken returns the token record, or nil if not found or expired.
func GetOAuthToken(ctx context.Context, q Querier, token string) (*model.OAuthToken, error) {
	var t model.OAuthToken
	var scopeStr string
	err := q.QueryRowContext(ctx,
		`SELECT token, user_id, client_id, scopes, created_at, expires_at
		 FROM oauth_tokens
		 WHERE token = ? AND (expires_at IS NULL OR expires_at > datetime('now'))`,
		token,
	).Scan(&t.Token, &t.UserID, &t.ClientID, &scopeStr, &t.CreatedAt, &t.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get oauth token: %w", err)
	}
	t.Scopes = strings.Fields(scopeStr)
	return &t, nil
}

// ListOAuthTokensByUserID returns all non-expired tokens for the given user.
func ListOAuthTokensByUserID(ctx context.Context, q Querier, userID int64) ([]*model.OAuthToken, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT token, user_id, client_id, scopes, created_at, expires_at
		 FROM oauth_tokens
		 WHERE user_id = ? AND (expires_at IS NULL OR expires_at > datetime('now'))
		 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("list oauth tokens by user: %w", err)
	}
	defer rows.Close()

	var tokens []*model.OAuthToken
	for rows.Next() {
		var t model.OAuthToken
		var scopeStr string
		if err := rows.Scan(&t.Token, &t.UserID, &t.ClientID, &scopeStr, &t.CreatedAt, &t.ExpiresAt); err != nil {
			return nil, fmt.Errorf("scan oauth token: %w", err)
		}
		t.Scopes = strings.Fields(scopeStr)
		tokens = append(tokens, &t)
	}
	return tokens, rows.Err()
}

// DeleteOAuthToken revokes a token.
func DeleteOAuthToken(ctx context.Context, q Querier, token string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM oauth_tokens WHERE token = ?", token)
	if err != nil {
		return fmt.Errorf("delete oauth token: %w", err)
	}
	return nil
}

// DeleteExpiredOAuthTokens removes all expired tokens.
func DeleteExpiredOAuthTokens(ctx context.Context, q Querier) error {
	_, err := q.ExecContext(ctx, "DELETE FROM oauth_tokens WHERE expires_at IS NOT NULL AND expires_at <= datetime('now')")
	if err != nil {
		return fmt.Errorf("delete expired oauth tokens: %w", err)
	}
	return nil
}

func oauthRandomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
