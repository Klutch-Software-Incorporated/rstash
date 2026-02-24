package handler

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"gosilo/internal/db"
	"gosilo/internal/ui"
)

type adminHandler struct {
	deps *UIDeps
}

// AdminHandler returns handler methods for admin pages.
func AdminHandler(deps *UIDeps) *adminHandler {
	return &adminHandler{deps: deps}
}

type dashboardContent struct {
	UserCount        int64
	RegistrationMode string
	BaseURL          string
	BlobBackend      string
}

func (h *adminHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	count, err := db.UserCount(r.Context(), h.deps.DB)
	if err != nil {
		slog.Error("failed to get user count", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	user := CurrentUser(r)
	h.deps.Renderer.Render(w, "admin_dashboard", ui.PageData{
		Title:            "Admin — Gosilo",
		CurrentUser:      userInfo(user),
		CSRFToken:        CSRFToken(r),
		Flash:            GetFlash(w, r),
		RegistrationMode: h.deps.Config.RegistrationMode,
		Content: &dashboardContent{
			UserCount:        count,
			RegistrationMode: h.deps.Config.RegistrationMode,
			BaseURL:          h.deps.Config.BaseURL,
			BlobBackend:      h.deps.Config.BlobBackend,
		},
	})
}

type usersContent struct {
	Users []*userRow
}

type userRow struct {
	ID        int64
	Username  string
	IsAdmin   bool
	CreatedAt string
	IsSelf    bool
}

func (h *adminHandler) Users(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	users, err := db.ListUsers(r.Context(), h.deps.DB)
	if err != nil {
		slog.Error("failed to list users", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	currentUser := CurrentUser(r)
	rows := make([]*userRow, len(users))
	for i, u := range users {
		rows[i] = &userRow{
			ID:        u.ID,
			Username:  u.Username,
			IsAdmin:   u.IsAdmin,
			CreatedAt: u.CreatedAt,
			IsSelf:    u.ID == currentUser.ID,
		}
	}

	h.deps.Renderer.Render(w, "admin_users", ui.PageData{
		Title:            "Users — Admin — Gosilo",
		CurrentUser:      userInfo(currentUser),
		CSRFToken:        CSRFToken(r),
		Flash:            GetFlash(w, r),
		RegistrationMode: h.deps.Config.RegistrationMode,
		Content:          &usersContent{Users: rows},
	})
}

func (h *adminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}
	if !ValidateCSRF(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	currentUser := CurrentUser(r)
	if id == currentUser.ID {
		SetFlash(w, "You cannot delete your own account.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	// Delete user's sessions first, then the user.
	if err := db.DeleteUserSessions(r.Context(), h.deps.DB, id); err != nil {
		slog.Error("failed to delete user sessions", "error", err)
	}
	if err := db.DeleteUser(r.Context(), h.deps.DB, id); err != nil {
		slog.Error("failed to delete user", "error", err)
		SetFlash(w, "Failed to delete user.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	SetFlash(w, fmt.Sprintf("User deleted."))
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

type oauthTestContent struct {
	CallbackURL string
	BaseURL     string
	Username    string
	Token       string
	TokenType   string
	State       string
	Error       string
}

func (h *adminHandler) OAuthTest(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	user := CurrentUser(r)
	callbackURL := h.deps.Config.BaseURL + "/admin/oauth-test"

	content := &oauthTestContent{
		CallbackURL: callbackURL,
		BaseURL:     h.deps.Config.BaseURL,
		Username:    user.Username,
	}

	// Check if this is a callback (fragment params forwarded as query params by JS).
	if r.URL.Query().Get("callback") == "1" {
		content.Token = r.URL.Query().Get("access_token")
		content.TokenType = r.URL.Query().Get("token_type")
		content.State = r.URL.Query().Get("state")
		content.Error = r.URL.Query().Get("error")
	}

	h.deps.Renderer.Render(w, "admin_oauth_test", ui.PageData{
		Title:            "OAuth Test — Admin — Gosilo",
		CurrentUser:      userInfo(user),
		CSRFToken:        CSRFToken(r),
		Flash:            GetFlash(w, r),
		RegistrationMode: h.deps.Config.RegistrationMode,
		Content:          content,
	})
}

type invitesContent struct {
	Invites []*inviteRow
}

type inviteRow struct {
	Code      string
	UsedBy    *int64
	CreatedAt string
}

func (h *adminHandler) Invites(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	invites, err := db.ListInviteCodes(r.Context(), h.deps.DB)
	if err != nil {
		slog.Error("failed to list invite codes", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	rows := make([]*inviteRow, len(invites))
	for i, inv := range invites {
		rows[i] = &inviteRow{
			Code:      inv.Code,
			UsedBy:    inv.UsedBy,
			CreatedAt: inv.CreatedAt,
		}
	}

	user := CurrentUser(r)
	h.deps.Renderer.Render(w, "admin_invites", ui.PageData{
		Title:            "Invites — Admin — Gosilo",
		CurrentUser:      userInfo(user),
		CSRFToken:        CSRFToken(r),
		Flash:            GetFlash(w, r),
		RegistrationMode: h.deps.Config.RegistrationMode,
		Content:          &invitesContent{Invites: rows},
	})
}

func (h *adminHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}
	if !ValidateCSRF(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	user := CurrentUser(r)
	inv, err := db.CreateInviteCode(r.Context(), h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to create invite code", "error", err)
		SetFlash(w, "Failed to generate invite code.")
		http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
		return
	}

	SetFlash(w, fmt.Sprintf("Invite code created: %s", inv.Code))
	http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
}

func (h *adminHandler) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}
	if !ValidateCSRF(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	code := r.PathValue("code")
	if err := db.DeleteInviteCode(r.Context(), h.deps.DB, code); err != nil {
		slog.Error("failed to delete invite code", "error", err)
		SetFlash(w, "Failed to delete invite code.")
		http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
		return
	}

	SetFlash(w, "Invite code deleted.")
	http.Redirect(w, r, "/admin/invites", http.StatusSeeOther)
}
