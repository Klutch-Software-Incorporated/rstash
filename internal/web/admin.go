package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

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
	// Activity
	ActiveUsers24h int64
	ActiveUsers7d  int64
	TopUsers       []*topUserRow
	// Users
	Users []*userRow
	// Invites
	Invites []*inviteRow
	// Audit log
	AuditLog []*auditRow
	// OAuth test
	OAuthTest *oauthTestContent
}

type userRow struct {
	ID           int64
	Username     string
	IsAdmin      bool
	Disabled     bool
	CreatedAt    string
	IsSelf       bool
	StorageUsed  string
	StorageQuota string // human-readable, empty if no quota
	SessionCount int64
}

type inviteRow struct {
	Code      string
	UsedBy    *int64
	CreatedAt string
}

type topUserRow struct {
	Username    string
	StorageUsed string
}

type auditRow struct {
	ActorUsername string
	Action       string
	TargetType   string
	TargetID     string
	Details      string
	CreatedAt    string
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

	// Activity: active user counts
	now := time.Now().UTC()
	since24h := now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	since7d := now.Add(-7 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	content.ActiveUsers24h, _ = db.ActiveUserCount(ctx, h.deps.DB, since24h)
	content.ActiveUsers7d, _ = db.ActiveUserCount(ctx, h.deps.DB, since7d)

	// Top users by storage
	topUsers, err := db.TopUsersByStorage(ctx, h.deps.DB, 5)
	if err != nil {
		slog.Error("failed to get top users by storage", "error", err)
	} else {
		topRows := make([]*topUserRow, len(topUsers))
		for i, t := range topUsers {
			topRows[i] = &topUserRow{
				Username:    t.Username,
				StorageUsed: formatBytes(t.StorageUsed),
			}
		}
		content.TopUsers = topRows
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
				Disabled:  u.Disabled,
				CreatedAt: u.CreatedAt,
				IsSelf:    u.ID == currentUser.ID,
			}
			sessCount, err := h.deps.Auth.CountUserSessions(ctx, u.ID)
			if err != nil {
				slog.Error("failed to count user sessions", "user_id", u.ID, "error", err)
			} else {
				row.SessionCount = sessCount
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

	// Audit log (last 25 entries)
	auditEntries, err := db.ListAuditEntries(ctx, h.deps.DB, 25, 0)
	if err != nil {
		slog.Error("failed to list audit entries", "error", err)
	} else {
		aRows := make([]*auditRow, len(auditEntries))
		for i, e := range auditEntries {
			aRows[i] = &auditRow{
				ActorUsername: e.ActorUsername,
				Action:       e.Action,
				TargetType:   e.TargetType,
				TargetID:     e.TargetID,
				Details:      e.Details,
				CreatedAt:    e.CreatedAt,
			}
		}
		content.AuditLog = aRows
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
		ui.SetFlashError(w, "You cannot delete your own account.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	target, _ := h.deps.Auth.GetUser(r.Context(), id)
	if err := h.deps.Auth.DeleteUser(r.Context(), id); err != nil {
		slog.Error("failed to delete user", "error", err)
		ui.SetFlashError(w, "Failed to delete user.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	targetName := idStr
	if target != nil {
		targetName = target.Username
	}
	h.audit(r, "user.deleted", "user", idStr, targetName)

	ui.SetFlash(w, "User deleted.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *adminHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	if h.deps.Config.RegistrationMode == "closed" {
		ui.SetFlashError(w, "Cannot create invite codes while registration is closed.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	user := CurrentUser(r)
	inv, err := h.deps.Auth.CreateInvite(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to create invite code", "error", err)
		ui.SetFlashError(w, "Failed to generate invite code.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	h.audit(r, "invite.created", "invite", inv.Code, "")

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
			ui.SetFlashError(w, fmt.Sprintf("Invalid quota value: %s", quotaStr))
			http.Redirect(w, r, "/admin", http.StatusSeeOther)
			return
		}
		quotaBytes = parsed
	}

	if err := db.UpdateUserQuota(r.Context(), h.deps.DB, id, quotaBytes); err != nil {
		slog.Error("failed to update user quota", "error", err)
		ui.SetFlashError(w, "Failed to update quota.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	h.audit(r, "user.quota_changed", "user", idStr, quotaStr)

	if quotaBytes > 0 {
		ui.SetFlash(w, fmt.Sprintf("Quota updated to %s.", formatBytes(quotaBytes)))
	} else {
		ui.SetFlash(w, "Quota reset to server default.")
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *adminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	username := r.FormValue("username")
	password := r.FormValue("password")
	isAdmin := r.FormValue("is_admin") == "on"

	if username == "" || password == "" {
		ui.SetFlashError(w, "Username and password are required.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	if msg := validatePassword(password); msg != "" {
		ui.SetFlashError(w, msg)
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	newUser, err := h.deps.Auth.CreateUser(r.Context(), username, password, isAdmin)
	if err != nil {
		slog.Error("failed to create user", "error", err)
		ui.SetFlashError(w, "Failed to create user. Username may already exist.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	h.audit(r, "user.created", "user", fmt.Sprintf("%d", newUser.ID), username)

	ui.SetFlash(w, fmt.Sprintf("User %q created.", username))
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *adminHandler) ToggleAdmin(w http.ResponseWriter, r *http.Request) {
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
		ui.SetFlashError(w, "You cannot change your own admin status.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	user, err := h.deps.Auth.GetUser(r.Context(), id)
	if err != nil || user == nil {
		ui.SetFlashError(w, "User not found.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	newAdmin := !user.IsAdmin
	if err := h.deps.Auth.ToggleAdmin(r.Context(), id, newAdmin); err != nil {
		slog.Error("failed to toggle admin", "error", err)
		ui.SetFlashError(w, "Failed to toggle admin status.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	h.audit(r, "user.admin_toggled", "user", idStr, fmt.Sprintf("admin=%v", newAdmin))

	if newAdmin {
		ui.SetFlash(w, fmt.Sprintf("%s is now an admin.", user.Username))
	} else {
		ui.SetFlash(w, fmt.Sprintf("%s is no longer an admin.", user.Username))
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

func (h *adminHandler) ToggleDisabled(w http.ResponseWriter, r *http.Request) {
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
		ui.SetFlashError(w, "You cannot disable your own account.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	user, err := h.deps.Auth.GetUser(r.Context(), id)
	if err != nil || user == nil {
		ui.SetFlashError(w, "User not found.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	newDisabled := !user.Disabled
	if err := h.deps.Auth.SetDisabled(r.Context(), id, newDisabled); err != nil {
		slog.Error("failed to toggle disabled", "error", err)
		ui.SetFlashError(w, "Failed to toggle disabled status.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	if newDisabled {
		if err := h.deps.Auth.TerminateAllSessions(r.Context(), id); err != nil {
			slog.Error("failed to terminate sessions on disable", "error", err)
		}
		h.audit(r, "user.disabled", "user", idStr, user.Username)
		ui.SetFlash(w, fmt.Sprintf("%s has been disabled.", user.Username))
	} else {
		h.audit(r, "user.enabled", "user", idStr, user.Username)
		ui.SetFlash(w, fmt.Sprintf("%s has been enabled.", user.Username))
	}
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

type adminSessionsContent struct {
	UserID   int64
	Username string
	Sessions []*adminSessionRow
}

type adminSessionRow struct {
	TokenPrefix string
	TokenFull   string
	CreatedAt   string
	ExpiresAt   string
}

func (h *adminHandler) UserSessions(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	user, err := h.deps.Auth.GetUser(r.Context(), id)
	if err != nil || user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	sessions, err := h.deps.Auth.ListUserSessions(r.Context(), id)
	if err != nil {
		slog.Error("failed to list user sessions", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	rows := make([]*adminSessionRow, len(sessions))
	for i, s := range sessions {
		prefix := s.Token[:8] + "..."
		rows[i] = &adminSessionRow{
			TokenPrefix: prefix,
			TokenFull:   s.Token,
			CreatedAt:   s.CreatedAt,
			ExpiresAt:   s.ExpiresAt,
		}
	}

	h.deps.Renderer.Render(w, "admin_sessions", h.deps.pageData(w, r, fmt.Sprintf("Sessions — %s", user.Username), adminSessionsContent{
		UserID:   user.ID,
		Username: user.Username,
		Sessions: rows,
	}))
}

func (h *adminHandler) TerminateSession(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	token := r.PathValue("token")
	if err := h.deps.Auth.TerminateSession(r.Context(), token); err != nil {
		slog.Error("failed to terminate session", "error", err)
		ui.SetFlashError(w, "Failed to terminate session.")
	} else {
		h.audit(r, "session.terminated", "session", token[:8]+"...", "")
		ui.SetFlash(w, "Session terminated.")
	}

	referer := r.Header.Get("Referer")
	if referer != "" {
		http.Redirect(w, r, referer, http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	}
}

func (h *adminHandler) TerminateAllSessions(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	if err := h.deps.Auth.TerminateAllSessions(r.Context(), id); err != nil {
		slog.Error("failed to terminate all sessions", "error", err)
		ui.SetFlashError(w, "Failed to terminate sessions.")
	} else {
		h.audit(r, "session.terminated", "user", idStr, "all sessions")
		ui.SetFlash(w, "All sessions terminated.")
	}

	http.Redirect(w, r, fmt.Sprintf("/admin/users/%d/sessions", id), http.StatusSeeOther)
}

type adminUserContent struct {
	UserID       int64
	Username     string
	IsAdmin      bool
	Disabled     bool
	CreatedAt    string
	StorageUsed  string
	FileCount    int64
	SessionCount int64
	RecentFiles  []*recentFileRow
	AuditLog     []*auditRow
}

type recentFileRow struct {
	Path        string
	Size        string
	ContentType string
	UpdatedAt   string
}

func (h *adminHandler) UserActivity(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	user, err := h.deps.Auth.GetUser(ctx, id)
	if err != nil || user == nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	stats, _ := db.GetUserStorageStats(ctx, h.deps.DB, id)
	sessCount, _ := h.deps.Auth.CountUserSessions(ctx, id)
	recentNodes, _ := db.GetRecentUserNodes(ctx, h.deps.DB, id, 10)

	fileRows := make([]*recentFileRow, len(recentNodes))
	for i, n := range recentNodes {
		fileRows[i] = &recentFileRow{
			Path:        n.Path,
			Size:        formatBytes(n.ContentLength),
			ContentType: n.ContentType,
			UpdatedAt:   n.UpdatedAt,
		}
	}

	auditEntries, _ := db.ListAuditEntriesByTarget(ctx, h.deps.DB, "user", idStr, 25)
	aRows := make([]*auditRow, len(auditEntries))
	for i, e := range auditEntries {
		aRows[i] = &auditRow{
			ActorUsername: e.ActorUsername,
			Action:       e.Action,
			TargetType:   e.TargetType,
			TargetID:     e.TargetID,
			Details:      e.Details,
			CreatedAt:    e.CreatedAt,
		}
	}

	content := &adminUserContent{
		UserID:       user.ID,
		Username:     user.Username,
		IsAdmin:      user.IsAdmin,
		Disabled:     user.Disabled,
		CreatedAt:    user.CreatedAt,
		SessionCount: sessCount,
		RecentFiles:  fileRows,
		AuditLog:     aRows,
	}
	if stats != nil {
		content.StorageUsed = formatBytes(stats.TotalBytes)
		content.FileCount = stats.FileCount
	}

	h.deps.Renderer.Render(w, "admin_user", h.deps.pageData(w, r, fmt.Sprintf("User — %s", user.Username), content))
}

func (h *adminHandler) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) || !RequireAdmin(w, r) {
		return
	}

	code := r.PathValue("code")
	if err := h.deps.Auth.DeleteInvite(r.Context(), code); err != nil {
		slog.Error("failed to delete invite code", "error", err)
		ui.SetFlashError(w, "Failed to delete invite code.")
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	h.audit(r, "invite.deleted", "invite", code, "")

	ui.SetFlash(w, "Invite code deleted.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}

// audit is a helper to log admin actions to the audit trail.
func (h *adminHandler) audit(r *http.Request, action, targetType, targetID, details string) {
	user := CurrentUser(r)
	if user == nil {
		return
	}
	if err := db.InsertAuditEntry(r.Context(), h.deps.DB, user.ID, action, targetType, targetID, details); err != nil {
		slog.Error("failed to write audit log", "error", err, "action", action)
	}
}
