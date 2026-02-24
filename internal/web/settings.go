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

// SettingsHandler returns handler methods for the account settings page.
func SettingsHandler(deps *UIDeps) *settingsHandler {
	return &settingsHandler{deps: deps}
}

type settingsContent struct {
	Tokens []*tokenRow
}

type changePasswordContent struct {
	Error string
}

type tokenRow struct {
	TokenPrefix string
	TokenFull   string
	ClientID    string
	Scopes      string
	CreatedAt   string
}

func (h *settingsHandler) ShowSettings(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}

	user := CurrentUser(r)
	tokens, err := db.ListOAuthTokensByUserID(r.Context(), h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to list oauth tokens", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	rows := make([]*tokenRow, len(tokens))
	for i, t := range tokens {
		rows[i] = &tokenRow{
			TokenPrefix: t.Token[:8] + "...",
			TokenFull:   t.Token,
			ClientID:    t.ClientID,
			Scopes:      strings.Join(t.Scopes, ", "),
			CreatedAt:   t.CreatedAt,
		}
	}

	h.deps.Renderer.Render(w, "settings", h.deps.pageData(w, r, "Settings — Gosilo", &settingsContent{Tokens: rows}))
}

func (h *settingsHandler) ShowChangePassword(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}

	h.deps.Renderer.Render(w, "change_password", h.deps.pageData(w, r, "Change Password — Gosilo", &changePasswordContent{}))
}

func (h *settingsHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}

	user := CurrentUser(r)
	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "change_password", ui.PageData{
			Title:            "Change Password — Gosilo",
			CurrentUser:      userInfo(user),
			CSRFToken:        CSRFToken(r),
			RegistrationMode: h.deps.Config.RegistrationMode,
			Content:          &changePasswordContent{Error: msg},
		})
	}

	if !h.deps.Auth.CheckPassword(user, currentPassword) {
		renderErr("Current password is incorrect.")
		return
	}

	if msg := validatePassword(newPassword); msg != "" {
		renderErr(msg)
		return
	}

	if newPassword != confirmPassword {
		renderErr("New passwords do not match.")
		return
	}

	if err := h.deps.Auth.UpdatePassword(r.Context(), user.ID, newPassword); err != nil {
		slog.Error("failed to update password", "error", err)
		renderErr("An error occurred. Please try again.")
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
