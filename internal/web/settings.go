package web

import (
	"log/slog"
	"net/http"

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
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if msg := validatePassword(newPassword); msg != "" {
		ui.SetFlash(w, msg)
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if newPassword != confirmPassword {
		ui.SetFlash(w, "New passwords do not match.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := h.deps.Auth.UpdatePassword(r.Context(), user.ID, newPassword); err != nil {
		slog.Error("failed to update password", "error", err)
		ui.SetFlash(w, "An error occurred. Please try again.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Invalidate all other sessions.
	sess := CurrentSession(r)
	if err := h.deps.Auth.InvalidateOtherSessions(r.Context(), user.ID, sess.Token); err != nil {
		slog.Error("failed to invalidate other sessions", "error", err)
	}

	ui.SetFlash(w, "Password changed successfully.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if t == nil || t.UserID != user.ID {
		ui.SetFlash(w, "Token not found.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := db.DeleteOAuthToken(r.Context(), h.deps.DB, token); err != nil {
		slog.Error("failed to revoke oauth token", "error", err)
		ui.SetFlash(w, "Failed to revoke token.")
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, "Token revoked.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
