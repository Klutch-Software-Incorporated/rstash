package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"rstash/internal/config"
	"rstash/internal/db"
)

// OAuthToken handles POST /oauth/token for the authorization code + PKCE exchange
// and the refresh_token grant type.
// tokenLifetimeFunc returns the current token_lifetime setting string.
// refreshFunc returns whether refresh tokens are enabled and their lifetime string.
func OAuthToken(repo *db.Repository, tokenLifetimeFunc func() string, refreshFunc func() (enabled bool, lifetime string)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			tokenError(w, "invalid_request", "method must be POST", http.StatusBadRequest)
			return
		}

		grantType := r.FormValue("grant_type")

		switch grantType {
		case "authorization_code":
			handleAuthorizationCode(w, r, repo, tokenLifetimeFunc, refreshFunc)
		case "refresh_token":
			handleRefreshToken(w, r, repo, tokenLifetimeFunc, refreshFunc)
		default:
			tokenError(w, "unsupported_grant_type", "supported grant types: authorization_code, refresh_token", http.StatusBadRequest)
		}
	})
}

func handleAuthorizationCode(w http.ResponseWriter, r *http.Request, repo *db.Repository, tokenLifetimeFunc func() string, refreshFunc func() (bool, string)) {
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")
	redirectURI := r.FormValue("redirect_uri")

	if code == "" || codeVerifier == "" || redirectURI == "" {
		tokenError(w, "invalid_request", "code, code_verifier, and redirect_uri are required", http.StatusBadRequest)
		return
	}

	// Look up the authorization code.
	ac, err := repo.GetAuthorizationCode(r.Context(), code)
	if err != nil {
		slog.Error("get authorization code", "error", err)
		tokenError(w, "server_error", "internal error", http.StatusInternalServerError)
		return
	}
	if ac == nil {
		tokenError(w, "invalid_grant", "authorization code is invalid, expired, or already used", http.StatusBadRequest)
		return
	}

	// Verify redirect_uri matches.
	if ac.RedirectURI != redirectURI {
		tokenError(w, "invalid_grant", "redirect_uri does not match", http.StatusBadRequest)
		return
	}

	// Verify PKCE: BASE64URL(SHA256(code_verifier)) must equal code_challenge.
	if !verifyPKCE(codeVerifier, ac.CodeChallenge) {
		tokenError(w, "invalid_grant", "code_verifier does not match code_challenge", http.StatusBadRequest)
		return
	}

	// Reject if user account is disabled.
	acUser, err := repo.GetUserByID(r.Context(), ac.UserID)
	if err != nil {
		slog.Error("get user for auth code", "error", err)
		tokenError(w, "server_error", "internal error", http.StatusInternalServerError)
		return
	}
	if acUser == nil || acUser.Disabled || !acUser.Approved {
		tokenError(w, "invalid_grant", "user account is disabled", http.StatusBadRequest)
		return
	}

	// Mark code as used.
	if err := repo.UseAuthorizationCode(r.Context(), code); err != nil {
		slog.Error("use authorization code", "error", err)
		tokenError(w, "server_error", "internal error", http.StatusInternalServerError)
		return
	}

	// Parse token lifetime setting.
	lifetimeStr := tokenLifetimeFunc()
	lifetime, _ := config.ParseTokenLifetime(lifetimeStr)

	// Create the access token.
	scopes := strings.Fields(ac.Scopes)
	token, err := repo.CreateOAuthToken(r.Context(), ac.UserID, ac.ClientID, scopes, lifetime)
	if err != nil {
		slog.Error("create oauth token", "error", err)
		tokenError(w, "server_error", "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	resp := map[string]any{
		"access_token": token.Token,
		"token_type":   "bearer",
	}
	if lifetime > 0 {
		resp["expires_in"] = int64(lifetime.Seconds())
	}

	// Issue refresh token if enabled.
	if refreshEnabled, refreshLifetimeStr := refreshFunc(); refreshEnabled {
		refreshLifetime, _ := config.ParseTokenLifetime(refreshLifetimeStr)
		rt, err := repo.CreateRefreshToken(r.Context(), ac.UserID, ac.ClientID, scopes, token.Token, refreshLifetime)
		if err != nil {
			slog.Error("create refresh token", "error", err)
		} else {
			resp["refresh_token"] = rt.Token
		}
	}

	json.NewEncoder(w).Encode(resp)
}

func handleRefreshToken(w http.ResponseWriter, r *http.Request, repo *db.Repository, tokenLifetimeFunc func() string, refreshFunc func() (bool, string)) {
	refreshToken := r.FormValue("refresh_token")
	if refreshToken == "" {
		tokenError(w, "invalid_request", "refresh_token is required", http.StatusBadRequest)
		return
	}

	// Look up the refresh token.
	rt, err := repo.GetRefreshToken(r.Context(), refreshToken)
	if err != nil {
		slog.Error("get refresh token", "error", err)
		tokenError(w, "server_error", "internal error", http.StatusInternalServerError)
		return
	}
	if rt == nil {
		tokenError(w, "invalid_grant", "refresh token is invalid or expired", http.StatusBadRequest)
		return
	}

	// Reject if user account is disabled, and revoke the refresh token.
	rtUser, err := repo.GetUserByID(r.Context(), rt.UserID)
	if err != nil {
		slog.Error("get user for refresh token", "error", err)
		tokenError(w, "server_error", "internal error", http.StatusInternalServerError)
		return
	}
	if rtUser == nil || rtUser.Disabled || !rtUser.Approved {
		_ = repo.DeleteRefreshToken(r.Context(), refreshToken)
		tokenError(w, "invalid_grant", "user account is disabled", http.StatusBadRequest)
		return
	}

	// Delete old access token and old refresh token (rotation).
	_ = repo.DeleteOAuthToken(r.Context(), rt.AccessToken)
	_ = repo.DeleteRefreshToken(r.Context(), refreshToken)

	// Create new access token.
	lifetimeStr := tokenLifetimeFunc()
	lifetime, _ := config.ParseTokenLifetime(lifetimeStr)

	newToken, err := repo.CreateOAuthToken(r.Context(), rt.UserID, rt.ClientID, strings.Fields(rt.Scopes), lifetime)
	if err != nil {
		slog.Error("create oauth token on refresh", "error", err)
		tokenError(w, "server_error", "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	resp := map[string]any{
		"access_token": newToken.Token,
		"token_type":   "bearer",
	}
	if lifetime > 0 {
		resp["expires_in"] = int64(lifetime.Seconds())
	}

	// Issue new refresh token if still enabled (rotation).
	if refreshEnabled, refreshLifetimeStr := refreshFunc(); refreshEnabled {
		refreshLifetime, _ := config.ParseTokenLifetime(refreshLifetimeStr)
		newRT, err := repo.CreateRefreshToken(r.Context(), rt.UserID, rt.ClientID, strings.Fields(rt.Scopes), newToken.Token, refreshLifetime)
		if err != nil {
			slog.Error("create refresh token on refresh", "error", err)
		} else {
			resp["refresh_token"] = newRT.Token
		}
	}

	json.NewEncoder(w).Encode(resp)
}

// verifyPKCE checks that BASE64URL(SHA256(verifier)) == challenge.
func verifyPKCE(verifier, challenge string) bool {
	h := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(h[:])
	return computed == challenge
}

// tokenError writes an OAuth token error response as JSON.
func tokenError(w http.ResponseWriter, code, description string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{
		"error":             code,
		"error_description": description,
	})
}
