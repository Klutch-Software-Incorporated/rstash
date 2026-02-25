package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"

	"gosilo/internal/db"
)

// OAuthRevoke handles POST /oauth/revoke per RFC 7009.
// It accepts a token and optional token_type_hint, and always returns 200 OK.
func OAuthRevoke(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		token := r.FormValue("token")
		if token == "" {
			http.Error(w, "token parameter is required", http.StatusBadRequest)
			return
		}

		hint := r.FormValue("token_type_hint")
		ctx := r.Context()

		if hint == "refresh_token" {
			// Try refresh token first, then access token.
			if revoked := revokeRefreshToken(ctx, database, token); !revoked {
				revokeAccessToken(ctx, database, token)
			}
		} else {
			// Try access token first, then refresh token.
			if revoked := revokeAccessToken(ctx, database, token); !revoked {
				revokeRefreshToken(ctx, database, token)
			}
		}

		// Per RFC 7009, always return 200 OK.
		w.WriteHeader(http.StatusOK)
	})
}

// revokeAccessToken tries to revoke an access token and cascade to its refresh token.
// Returns true if the token was found.
func revokeAccessToken(ctx context.Context, database *sql.DB, token string) bool {
	t, err := db.GetOAuthTokenUnfiltered(ctx, database, token)
	if err != nil {
		slog.Error("revoke: get access token", "error", err)
		return false
	}
	if t == nil {
		return false
	}

	if err := db.DeleteOAuthToken(ctx, database, token); err != nil {
		slog.Error("revoke: delete access token", "error", err)
	}
	// Cascade: also delete any linked refresh token.
	if err := db.DeleteRefreshTokenByAccessToken(ctx, database, token); err != nil {
		slog.Error("revoke: cascade delete refresh token", "error", err)
	}

	db.Audit(ctx, database, t.UserID, "oauth.token_revoked", "token", token[:8]+"...", "via /oauth/revoke")
	return true
}

// revokeRefreshToken tries to revoke a refresh token and cascade to its access token.
// Returns true if the token was found.
func revokeRefreshToken(ctx context.Context, database *sql.DB, token string) bool {
	rt, err := db.GetRefreshTokenUnfiltered(ctx, database, token)
	if err != nil {
		slog.Error("revoke: get refresh token", "error", err)
		return false
	}
	if rt == nil {
		return false
	}

	if err := db.DeleteRefreshToken(ctx, database, token); err != nil {
		slog.Error("revoke: delete refresh token", "error", err)
	}
	// Cascade: also delete the linked access token.
	if err := db.DeleteOAuthToken(ctx, database, rt.AccessToken); err != nil {
		slog.Error("revoke: cascade delete access token", "error", err)
	}

	db.Audit(ctx, database, rt.UserID, "oauth.refresh_revoked", "token", token[:8]+"...", "via /oauth/revoke")
	return true
}
