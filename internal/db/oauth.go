package db

import (
	"context"
	"database/sql"
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
	token, err := RandomHex(32)
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

// CountUserOAuthTokens returns the count of active (non-expired) OAuth tokens
// for a user.
func CountUserOAuthTokens(ctx context.Context, q Querier, userID int64) (int64, error) {
	var count int64
	err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM oauth_tokens
		 WHERE user_id = ? AND (expires_at IS NULL OR expires_at > datetime('now'))`,
		userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count user oauth tokens: %w", err)
	}
	return count, nil
}

// GetRecentUserOAuthTokens returns recently created OAuth tokens for a user,
// ordered by creation time descending.
func GetRecentUserOAuthTokens(ctx context.Context, q Querier, userID int64, limit int) ([]*model.OAuthToken, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT token, user_id, client_id, scopes, created_at, expires_at
		 FROM oauth_tokens
		 WHERE user_id = ?
		 ORDER BY created_at DESC
		 LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recent user oauth tokens: %w", err)
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

// CreateAuthorizationCode generates a new authorization code for the OAuth code flow.
func CreateAuthorizationCode(ctx context.Context, q Querier, userID int64, clientID, redirectURI, scopes, codeChallenge, codeChallengeMethod string) (*model.AuthorizationCode, error) {
	code, err := RandomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate authorization code: %w", err)
	}

	var ac model.AuthorizationCode
	var used int
	err = q.QueryRowContext(ctx,
		`INSERT INTO authorization_codes (code, user_id, client_id, redirect_uri, scopes, code_challenge, code_challenge_method)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 RETURNING code, user_id, client_id, redirect_uri, scopes, code_challenge, code_challenge_method, created_at, expires_at, used`,
		code, userID, clientID, redirectURI, scopes, codeChallenge, codeChallengeMethod,
	).Scan(&ac.Code, &ac.UserID, &ac.ClientID, &ac.RedirectURI, &ac.Scopes, &ac.CodeChallenge, &ac.CodeChallengeMethod, &ac.CreatedAt, &ac.ExpiresAt, &used)
	if err != nil {
		return nil, fmt.Errorf("create authorization code: %w", err)
	}
	ac.Used = used != 0
	return &ac, nil
}

// GetAuthorizationCode returns the authorization code record, or nil if not found, expired, or already used.
func GetAuthorizationCode(ctx context.Context, q Querier, code string) (*model.AuthorizationCode, error) {
	var ac model.AuthorizationCode
	var used int
	err := q.QueryRowContext(ctx,
		`SELECT code, user_id, client_id, redirect_uri, scopes, code_challenge, code_challenge_method, created_at, expires_at, used
		 FROM authorization_codes
		 WHERE code = ? AND used = 0 AND expires_at > datetime('now')`,
		code,
	).Scan(&ac.Code, &ac.UserID, &ac.ClientID, &ac.RedirectURI, &ac.Scopes, &ac.CodeChallenge, &ac.CodeChallengeMethod, &ac.CreatedAt, &ac.ExpiresAt, &used)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get authorization code: %w", err)
	}
	ac.Used = used != 0
	return &ac, nil
}

// UseAuthorizationCode marks an authorization code as used (single-use enforcement).
func UseAuthorizationCode(ctx context.Context, q Querier, code string) error {
	_, err := q.ExecContext(ctx, "UPDATE authorization_codes SET used = 1 WHERE code = ?", code)
	if err != nil {
		return fmt.Errorf("use authorization code: %w", err)
	}
	return nil
}

// DeleteExpiredAuthorizationCodes removes expired or used authorization codes.
func DeleteExpiredAuthorizationCodes(ctx context.Context, q Querier) error {
	_, err := q.ExecContext(ctx, "DELETE FROM authorization_codes WHERE used = 1 OR expires_at <= datetime('now')")
	if err != nil {
		return fmt.Errorf("delete expired authorization codes: %w", err)
	}
	return nil
}
