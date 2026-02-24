package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"

	"gosilo/internal/model"
)

// CreateSession generates a new session token and CSRF token for the given user,
// with a 7-day expiry.
func CreateSession(ctx context.Context, q Querier, userID int64) (*model.Session, error) {
	token, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate session token: %w", err)
	}
	csrf, err := randomHex(32)
	if err != nil {
		return nil, fmt.Errorf("generate csrf token: %w", err)
	}

	var s model.Session
	err = q.QueryRowContext(ctx,
		`INSERT INTO sessions (token, user_id, csrf_token, expires_at)
		 VALUES (?, ?, ?, datetime('now', '+7 days'))
		 RETURNING token, user_id, csrf_token, created_at, expires_at`,
		token, userID, csrf,
	).Scan(&s.Token, &s.UserID, &s.CSRFToken, &s.CreatedAt, &s.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}
	return &s, nil
}

// GetSessionByToken returns the session with the given token, or nil if not found
// or expired.
func GetSessionByToken(ctx context.Context, q Querier, token string) (*model.Session, error) {
	var s model.Session
	err := q.QueryRowContext(ctx,
		`SELECT token, user_id, csrf_token, created_at, expires_at
		 FROM sessions
		 WHERE token = ? AND expires_at > datetime('now')`,
		token,
	).Scan(&s.Token, &s.UserID, &s.CSRFToken, &s.CreatedAt, &s.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &s, nil
}

// DeleteSession removes a session by token.
func DeleteSession(ctx context.Context, q Querier, token string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE token = ?", token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteExpiredSessions removes all expired sessions.
func DeleteExpiredSessions(ctx context.Context, q Querier) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= datetime('now')")
	if err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}
	return nil
}

// DeleteUserSessionsExcept removes all sessions for a user except the given token.
func DeleteUserSessionsExcept(ctx context.Context, q Querier, userID int64, exceptToken string) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ? AND token != ?", userID, exceptToken)
	if err != nil {
		return fmt.Errorf("delete user sessions except: %w", err)
	}
	return nil
}

// DeleteUserSessions removes all sessions for a given user.
func DeleteUserSessions(ctx context.Context, q Querier, userID int64) error {
	_, err := q.ExecContext(ctx, "DELETE FROM sessions WHERE user_id = ?", userID)
	if err != nil {
		return fmt.Errorf("delete user sessions: %w", err)
	}
	return nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
