package web

import (
	"log/slog"
	"net/http"
	"strings"

	"gosilo/internal/db"
	"gosilo/internal/ui"
)

type settingsHandler struct {
	deps *UIDeps
}

// SettingsHandler returns handler methods for account settings actions.
func SettingsHandler(deps *UIDeps) *settingsHandler {
	return &settingsHandler{deps: deps}
}

type tokenRow struct {
	TokenPrefix string
	TokenFull   string
	ClientID    string
	Scopes      string
	CreatedAt   string
}

type settingsContent struct {
	BaseURL   string
	Username  string
	CreatedAt string
	Tokens    []*tokenRow
	Sessions  []*sessionRow
	QuotaMode string
	Stats     *homeStats
}

type sessionRow struct {
	TokenPrefix string
	TokenFull   string
	CreatedAt   string
	ExpiresAt   string
	IsCurrent   bool
}

func (h *settingsHandler) Show(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}

	user := CurrentUser(r)
	sess := CurrentSession(r)
	ctx := r.Context()

	// Storage stats.
	stats, err := db.GetUserStorageStats(ctx, h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to get storage stats", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	hs := &homeStats{
		FileCount:    stats.FileCount,
		StorageUsed:  formatBytes(stats.TotalBytes),
		StorageBytes: stats.TotalBytes,
	}
	if h.deps.Config.QuotaMode == "user" {
		limit := h.deps.Config.QuotaUser
		if user.StorageQuota > 0 {
			limit = user.StorageQuota
		}
		hs.StorageQuota = formatBytes(limit)
		hs.QuotaBytes = limit
		if limit > 0 {
			pct := int(stats.TotalBytes * 100 / limit)
			if pct > 100 {
				pct = 100
			}
			hs.QuotaPercent = pct
		}
	}

	// OAuth tokens.
	oauthTokens, err := db.ListOAuthTokensByUserID(ctx, h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to list oauth tokens", "error", err)
		oauthTokens = nil
	}
	tokenRows := make([]*tokenRow, len(oauthTokens))
	for i, t := range oauthTokens {
		tokenRows[i] = &tokenRow{
			TokenPrefix: t.Token[:8] + "...",
			TokenFull:   t.Token,
			ClientID:    t.ClientID,
			Scopes:      strings.Join(t.Scopes, ", "),
			CreatedAt:   t.CreatedAt,
		}
	}

	// Sessions.
	sessions, err := h.deps.Auth.ListUserSessions(ctx, user.ID)
	if err != nil {
		slog.Error("failed to list user sessions", "error", err)
		sessions = nil
	}
	sessRows := make([]*sessionRow, len(sessions))
	for i, s := range sessions {
		sessRows[i] = &sessionRow{
			TokenPrefix: s.Token[:8] + "...",
			TokenFull:   s.Token,
			CreatedAt:   s.CreatedAt,
			ExpiresAt:   s.ExpiresAt,
			IsCurrent:   sess != nil && s.Token == sess.Token,
		}
	}

	tokenCount, err := db.CountUserOAuthTokens(ctx, h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to count oauth tokens", "error", err)
	}
	hs.OAuthApps = tokenCount

	h.deps.Renderer.Render(w, "settings", h.deps.pageData(w, r, "Settings — Gosilo", &settingsContent{
		BaseURL:   h.deps.Config.BaseURL,
		Username:  user.Username,
		CreatedAt: user.CreatedAt,
		Tokens:    tokenRows,
		Sessions:  sessRows,
		QuotaMode: h.deps.Config.QuotaMode,
		Stats:     hs,
	}))
}

func (h *settingsHandler) TerminateOwnSession(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}

	token := r.PathValue("token")
	sess := CurrentSession(r)

	if sess != nil && token == sess.Token {
		ui.SetFlash(w, "You cannot terminate your current session.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Verify the session belongs to the current user.
	targetSess, err := h.deps.Auth.GetSession(r.Context(), token)
	if err != nil || targetSess == nil || targetSess.UserID != CurrentUser(r).ID {
		ui.SetFlash(w, "Session not found.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if err := h.deps.Auth.TerminateSession(r.Context(), token); err != nil {
		slog.Error("failed to terminate session", "error", err)
		ui.SetFlash(w, "Failed to terminate session.")
	} else {
		ui.SetFlash(w, "Session terminated.")
	}

	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *settingsHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}

	user := CurrentUser(r)
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	if !h.deps.Auth.CheckPassword(user, currentPassword) {
		ui.SetFlash(w, "Current password is incorrect.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if msg := validatePassword(newPassword); msg != "" {
		ui.SetFlash(w, msg)
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if newPassword != confirmPassword {
		ui.SetFlash(w, "New passwords do not match.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if err := h.deps.Auth.UpdatePassword(r.Context(), user.ID, newPassword); err != nil {
		slog.Error("failed to update password", "error", err)
		ui.SetFlash(w, "An error occurred. Please try again.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	// Invalidate all other sessions.
	sess := CurrentSession(r)
	if err := h.deps.Auth.InvalidateOtherSessions(r.Context(), user.ID, sess.Token); err != nil {
		slog.Error("failed to invalidate other sessions", "error", err)
	}

	ui.SetFlash(w, "Password changed successfully.")
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (h *settingsHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}

	token := r.PathValue("token")
	user := CurrentUser(r)

	// Verify the token belongs to this user before deleting.
	t, err := db.GetOAuthToken(r.Context(), h.deps.DB, token)
	if err != nil {
		slog.Error("failed to get oauth token", "error", err)
		ui.SetFlash(w, "Failed to revoke token.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	if t == nil || t.UserID != user.ID {
		ui.SetFlash(w, "Token not found.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	if err := db.DeleteOAuthToken(r.Context(), h.deps.DB, token); err != nil {
		slog.Error("failed to revoke oauth token", "error", err)
		ui.SetFlash(w, "Failed to revoke token.")
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, "Token revoked.")
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
