package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"gosilo/internal/config"
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

type adminContent struct {
	// Server stats
	UserCount        int64
	RegistrationMode string
	BaseURL          string
	BlobBackend      string
	TotalStorageUsed string
	QuotaMode        string
	QuotaLimit       string
	// Users
	Users []*userRow
	// Invites
	Invites []*inviteRow
	// OAuth test
	OAuthTest *oauthTestContent
}

type userRow struct {
	ID           int64
	Username     string
	IsAdmin      bool
	CreatedAt    string
	IsSelf       bool
	StorageUsed  string
	StorageQuota string // human-readable, empty if no quota
}

type inviteRow struct {
	Code      string
	UsedBy    *int64
	CreatedAt string
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

// Show handles GET /admin — combined admin page with all sections.
func (h *adminHandler) Show(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	ctx := r.Context()
	currentUser := CurrentUser(r)

	// Server stats
	count, err := h.deps.Auth.UserCount(ctx)
	if err != nil {
		slog.Error("failed to get user count", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	totalUsed, err := db.GetTotalStorageUsed(ctx, h.deps.DB)
	if err != nil {
		slog.Error("failed to get total storage used", "error", err)
		totalUsed = 0
	}

	content := &adminContent{
		UserCount:        count,
		RegistrationMode: h.deps.Config.RegistrationMode,
		BaseURL:          h.deps.Config.BaseURL,
		BlobBackend:      h.deps.Config.BlobBackend,
		TotalStorageUsed: formatBytes(totalUsed),
		QuotaMode:        h.deps.Config.QuotaMode,
	}
	if h.deps.Config.QuotaMode == "total" {
		content.QuotaLimit = formatBytes(h.deps.Config.QuotaTotal)
	} else if h.deps.Config.QuotaMode == "user" {
		content.QuotaLimit = formatBytes(h.deps.Config.QuotaUser) + " per user"
	}

	// Users
	users, err := h.deps.Auth.ListUsers(ctx)
	if err != nil {
		slog.Error("failed to list users", "error", err)
	} else {
		rows := make([]*userRow, len(users))
		for i, u := range users {
			row := &userRow{
				ID:        u.ID,
				Username:  u.Username,
				IsAdmin:   u.IsAdmin,
				CreatedAt: u.CreatedAt,
				IsSelf:    u.ID == currentUser.ID,
			}
			stats, err := db.GetUserStorageStats(ctx, h.deps.DB, u.ID)
			if err != nil {
				slog.Error("failed to get user storage stats", "user_id", u.ID, "error", err)
			} else {
				row.StorageUsed = formatBytes(stats.TotalBytes)
			}
			if h.deps.Config.QuotaMode == "user" {
				if u.StorageQuota > 0 {
					row.StorageQuota = formatBytes(u.StorageQuota)
				} else {
					row.StorageQuota = formatBytes(h.deps.Config.QuotaUser) + " (default)"
				}
			}
			rows[i] = row
		}
		content.Users = rows
	}

	// Invites
	invites, err := h.deps.Auth.ListInvites(ctx)
	if err != nil {
		slog.Error("failed to list invite codes", "error", err)
	} else {
		rows := make([]*inviteRow, len(invites))
		for i, inv := range invites {
			rows[i] = &inviteRow{
				Code:      inv.Code,
				UsedBy:    inv.UsedBy,
				CreatedAt: inv.CreatedAt,
			}
		}
		content.Invites = rows
	}

	// OAuth test
	callbackURL := h.deps.Config.BaseURL + "/admin"
	oauthTest := &oauthTestContent{
		CallbackURL: callbackURL,
		BaseURL:     h.deps.Config.BaseURL,
		Username:    currentUser.Username,
	}
	if r.URL.Query().Get("callback") == "1" {
		oauthTest.Token = r.URL.Query().Get("access_token")
		oauthTest.TokenType = r.URL.Query().Get("token_type")
		oauthTest.State = r.URL.Query().Get("state")
		oauthTest.Error = r.URL.Query().Get("error")
	}
	content.OAuthTest = oauthTest

	h.deps.Renderer.Render(w, "admin", h.deps.pageData(w, r, "Admin — Gosilo", content))
}

func (h *adminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
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
		ui.SetFlash(w, "You cannot delete your own account.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	if err := h.deps.Auth.DeleteUser(r.Context(), id); err != nil {
		slog.Error("failed to delete user", "error", err)
		ui.SetFlash(w, "Failed to delete user.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, "User deleted.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *adminHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	user := CurrentUser(r)
	inv, err := h.deps.Auth.CreateInvite(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to create invite code", "error", err)
		ui.SetFlash(w, "Failed to generate invite code.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, fmt.Sprintf("Invite code created: %s", inv.Code))
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *adminHandler) SetUserQuota(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	quotaStr := r.FormValue("quota")
	var quotaBytes int64
	if quotaStr != "" && quotaStr != "0" {
		parsed, err := config.ParseByteSize(quotaStr)
		if err != nil {
			ui.SetFlash(w, fmt.Sprintf("Invalid quota value: %s", quotaStr))
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		quotaBytes = parsed
	}

	if err := db.UpdateUserQuota(r.Context(), h.deps.DB, id, quotaBytes); err != nil {
		slog.Error("failed to update user quota", "error", err)
		ui.SetFlash(w, "Failed to update quota.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	if quotaBytes > 0 {
		ui.SetFlash(w, fmt.Sprintf("Quota updated to %s.", formatBytes(quotaBytes)))
	} else {
		ui.SetFlash(w, "Quota reset to server default.")
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *adminHandler) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	code := r.PathValue("code")
	if err := h.deps.Auth.DeleteInvite(r.Context(), code); err != nil {
		slog.Error("failed to delete invite code", "error", err)
		ui.SetFlash(w, "Failed to delete invite code.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	ui.SetFlash(w, "Invite code deleted.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
