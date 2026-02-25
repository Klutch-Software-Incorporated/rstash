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

// --- Content structs (one per sub-page) ---

type adminDashboardContent struct {
	UserCount        int64
	RegistrationMode string
	BaseURL          string
	BlobDSN          string
	TotalStorageUsed string
	QuotaMode        string
	QuotaLimit       string
	ActiveUsers24h   int64
	ActiveUsers7d    int64
	TopUsers         []*topUserRow
}

type adminUsersContent struct {
	QuotaMode string
	Users     []*userRow
}

type adminSettingsContent struct {
	Groups  []adminSettingGroup  // runtime-editable, grouped
	EnvOnly []*adminSettingRow   // env-only (read-only)
}

type adminSettingGroup struct {
	Title    string
	Settings []*adminSettingRow
}

type adminAuditContent struct {
	AuditLog []*auditRow
}

type adminSettingRow struct {
	Key             string
	Label           string
	Description     string
	Value           string
	IsOverride      bool   // true if set in DB (not env default)
	RuntimeEditable bool   // can be changed at runtime via DB
	InputType       string // "text", "number", "select", "bytesize", "duration"
	ValidValues     []string
	NumberMin       string
	NumberStep      string
}

type userRow struct {
	ID           int64
	Username     string
	IsAdmin      bool
	Disabled     bool
	CreatedAt    string
	LastLoginAt  string
	LastLoginIP  string
	IsSelf       bool
	StorageUsed  string
	StorageQuota string // human-readable, empty if no quota
	SessionCount int64
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

// --- Handlers for each admin sub-page ---

// ShowDashboard handles GET /admin — server stats + top users.
func (h *adminHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()

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

	snap := h.deps.Settings.Load()
	content := &adminDashboardContent{
		UserCount:        count,
		RegistrationMode: snap.RegistrationMode,
		BaseURL:          h.deps.Config.BaseURL,
		BlobDSN:          h.deps.Config.BlobDSN,
		TotalStorageUsed: formatBytes(totalUsed),
		QuotaMode:        snap.QuotaMode,
	}
	if snap.QuotaMode == "total" {
		content.QuotaLimit = formatBytes(snap.QuotaTotal)
	} else if snap.QuotaMode == "user" {
		content.QuotaLimit = formatBytes(snap.QuotaUser) + " per user"
	}

	now := time.Now().UTC()
	since24h := now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05")
	since7d := now.Add(-7 * 24 * time.Hour).Format("2006-01-02 15:04:05")

	content.ActiveUsers24h, _ = db.ActiveUserCount(ctx, h.deps.DB, since24h)
	content.ActiveUsers7d, _ = db.ActiveUserCount(ctx, h.deps.DB, since7d)

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

	h.deps.Renderer.Render(w, "admin_dashboard", h.deps.adminPageData(w, r, "Admin — Gosilo", "overview", content))
}

// ShowUsers handles GET /admin/users — user list with actions.
func (h *adminHandler) ShowUsers(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	currentUser := CurrentUser(r)
	snap := h.deps.Settings.Load()

	users, err := h.deps.Auth.ListUsers(ctx)
	if err != nil {
		slog.Error("failed to list users", "error", err)
	}

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
		if u.LastLoginAt != nil {
			row.LastLoginAt = *u.LastLoginAt
		}
		if u.LastLoginIP != nil {
			row.LastLoginIP = *u.LastLoginIP
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
		if snap.QuotaMode == "user" {
			if u.StorageQuota > 0 {
				row.StorageQuota = formatBytes(u.StorageQuota)
			} else {
				row.StorageQuota = formatBytes(snap.QuotaUser) + " (default)"
			}
		}
		rows[i] = row
	}

	content := &adminUsersContent{
		QuotaMode: snap.QuotaMode,
		Users:     rows,
	}

	h.deps.Renderer.Render(w, "admin_users", h.deps.adminPageData(w, r, "Users — Admin", "users", content))
}

// ShowSettings handles GET /admin/settings — settings grid.
func (h *adminHandler) ShowSettings(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	snap := h.deps.Settings.Load()

	overrides, err := h.deps.Settings.Overrides(ctx)
	if err != nil {
		slog.Error("failed to load setting overrides", "error", err)
		overrides = map[string]string{}
	}

	// Merge runtime values (from snapshot) with env-only values (from config).
	runtimeValues := snap.ValueMap()
	envValues := h.deps.Config.ValueMap()

	// Build grouped runtime settings + flat env-only list.
	groupMap := make(map[string]*adminSettingGroup)
	var groupOrder []string
	var envOnly []*adminSettingRow

	for _, def := range config.SettingDefs() {
		value := envValues[def.Key]
		if v, ok := runtimeValues[def.Key]; ok {
			value = v
		}
		_, isOverride := overrides[def.Key]
		row := &adminSettingRow{
			Key:             def.Key,
			Label:           def.Label,
			Description:     def.Description,
			Value:           value,
			IsOverride:      isOverride,
			RuntimeEditable: def.RuntimeEditable,
			InputType:       string(def.InputType),
			ValidValues:     def.ValidValues,
			NumberMin:       def.NumberMin,
			NumberStep:      def.NumberStep,
		}

		if !def.RuntimeEditable {
			envOnly = append(envOnly, row)
			continue
		}

		g, ok := groupMap[def.Group]
		if !ok {
			g = &adminSettingGroup{Title: def.Group}
			groupMap[def.Group] = g
			groupOrder = append(groupOrder, def.Group)
		}
		g.Settings = append(g.Settings, row)
	}

	var groups []adminSettingGroup
	for _, name := range groupOrder {
		groups = append(groups, *groupMap[name])
	}

	content := &adminSettingsContent{Groups: groups, EnvOnly: envOnly}
	h.deps.Renderer.Render(w, "admin_settings", h.deps.adminPageData(w, r, "Settings — Admin", "settings", content))
}

// ShowAudit handles GET /admin/audit — audit log.
func (h *adminHandler) ShowAudit(w http.ResponseWriter, r *http.Request) {

	auditEntries, err := db.ListAuditEntries(r.Context(), h.deps.DB, 25, 0)
	if err != nil {
		slog.Error("failed to list audit entries", "error", err)
	}

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

	content := &adminAuditContent{AuditLog: aRows}
	h.deps.Renderer.Render(w, "admin_audit", h.deps.adminPageData(w, r, "Audit Log — Admin", "audit", content))
}

// ShowOAuthTest handles GET /admin/oauth-test — OAuth implicit flow test tool.
func (h *adminHandler) ShowOAuthTest(w http.ResponseWriter, r *http.Request) {

	currentUser := CurrentUser(r)
	callbackURL := h.deps.Config.BaseURL + "/admin/oauth-test"
	content := &oauthTestContent{
		CallbackURL: callbackURL,
		BaseURL:     h.deps.Config.BaseURL,
		Username:    currentUser.Username,
	}
	if r.URL.Query().Get("callback") == "1" {
		content.Token = r.URL.Query().Get("access_token")
		content.TokenType = r.URL.Query().Get("token_type")
		content.State = r.URL.Query().Get("state")
		content.Error = r.URL.Query().Get("error")
	}

	h.deps.Renderer.Render(w, "admin_oauth_test", h.deps.adminPageData(w, r, "OAuth Test — Admin", "oauth-test", content))
}

// --- POST handlers (redirects updated to sub-pages) ---

func (h *adminHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	currentUser := CurrentUser(r)
	if id == currentUser.ID {
		ui.SetFlashError(w, "You cannot delete your own account.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	target, _ := h.deps.Auth.GetUser(r.Context(), id)
	if err := h.deps.Auth.DeleteUser(r.Context(), id); err != nil {
		slog.Error("failed to delete user", "error", err)
		ui.SetFlashError(w, "Failed to delete user.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	targetName := idStr
	if target != nil {
		targetName = target.Username
	}
	h.audit(r, "user.deleted", "user", idStr, targetName)

	ui.SetFlash(w, "User deleted.")
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *adminHandler) SetUserQuota(w http.ResponseWriter, r *http.Request) {

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
			http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
			return
		}
		quotaBytes = parsed
	}

	if err := db.UpdateUserQuota(r.Context(), h.deps.DB, id, quotaBytes); err != nil {
		slog.Error("failed to update user quota", "error", err)
		ui.SetFlashError(w, "Failed to update quota.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	h.audit(r, "user.quota_changed", "user", idStr, quotaStr)

	if quotaBytes > 0 {
		ui.SetFlash(w, fmt.Sprintf("Quota updated to %s.", formatBytes(quotaBytes)))
	} else {
		ui.SetFlash(w, "Quota reset to server default.")
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *adminHandler) CreateUser(w http.ResponseWriter, r *http.Request) {

	username := r.FormValue("username")
	password := r.FormValue("password")
	isAdmin := r.FormValue("is_admin") == "on"

	if username == "" || password == "" {
		ui.SetFlashError(w, "Username and password are required.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	if msg := validatePassword(password); msg != "" {
		ui.SetFlashError(w, msg)
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	newUser, err := h.deps.Auth.CreateUser(r.Context(), username, password, isAdmin)
	if err != nil {
		slog.Error("failed to create user", "error", err)
		ui.SetFlashError(w, "Failed to create user. Username may already exist.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	h.audit(r, "user.created", "user", fmt.Sprintf("%d", newUser.ID), username)

	ui.SetFlash(w, fmt.Sprintf("User %q created.", username))
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *adminHandler) ToggleAdmin(w http.ResponseWriter, r *http.Request) {

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	currentUser := CurrentUser(r)
	if id == currentUser.ID {
		ui.SetFlashError(w, "You cannot change your own admin status.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	user, err := h.deps.Auth.GetUser(r.Context(), id)
	if err != nil || user == nil {
		ui.SetFlashError(w, "User not found.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	newAdmin := !user.IsAdmin
	if err := h.deps.Auth.ToggleAdmin(r.Context(), id, newAdmin); err != nil {
		slog.Error("failed to toggle admin", "error", err)
		ui.SetFlashError(w, "Failed to toggle admin status.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	h.audit(r, "user.admin_toggled", "user", idStr, fmt.Sprintf("admin=%v", newAdmin))

	if newAdmin {
		ui.SetFlash(w, fmt.Sprintf("%s is now an admin.", user.Username))
	} else {
		ui.SetFlash(w, fmt.Sprintf("%s is no longer an admin.", user.Username))
	}
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
}

func (h *adminHandler) ToggleDisabled(w http.ResponseWriter, r *http.Request) {

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid user ID", http.StatusBadRequest)
		return
	}

	currentUser := CurrentUser(r)
	if id == currentUser.ID {
		ui.SetFlashError(w, "You cannot disable your own account.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	user, err := h.deps.Auth.GetUser(r.Context(), id)
	if err != nil || user == nil {
		ui.SetFlashError(w, "User not found.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
		return
	}

	newDisabled := !user.Disabled
	if err := h.deps.Auth.SetDisabled(r.Context(), id, newDisabled); err != nil {
		slog.Error("failed to toggle disabled", "error", err)
		ui.SetFlashError(w, "Failed to toggle disabled status.")
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
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
	http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
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

	h.deps.Renderer.Render(w, "admin_sessions", h.deps.adminPageData(w, r, fmt.Sprintf("Sessions — %s", user.Username), "users", adminSessionsContent{
		UserID:   user.ID,
		Username: user.Username,
		Sessions: rows,
	}))
}

func (h *adminHandler) TerminateSession(w http.ResponseWriter, r *http.Request) {

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
		http.Redirect(w, r, "/admin/users", http.StatusSeeOther)
	}
}

func (h *adminHandler) TerminateAllSessions(w http.ResponseWriter, r *http.Request) {

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
	LastLoginAt  string
	LastLoginIP  string
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
	if user.LastLoginAt != nil {
		content.LastLoginAt = *user.LastLoginAt
	}
	if user.LastLoginIP != nil {
		content.LastLoginIP = *user.LastLoginIP
	}
	if stats != nil {
		content.StorageUsed = formatBytes(stats.TotalBytes)
		content.FileCount = stats.FileCount
	}

	h.deps.Renderer.Render(w, "admin_user", h.deps.adminPageData(w, r, fmt.Sprintf("User — %s", user.Username), "users", content))
}

// UpdateSettings handles POST /admin/settings — update runtime settings.
func (h *adminHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	changed := 0
	snap := h.deps.Settings.Load()
	currentValues := snap.ValueMap()

	for _, def := range config.RuntimeSettingDefs() {
		newVal := r.FormValue(def.Key)
		if newVal == "" {
			continue
		}

		if newVal == currentValues[def.Key] {
			continue
		}

		if err := h.deps.Settings.Set(ctx, def.Key, newVal); err != nil {
			slog.Error("failed to update setting", "key", def.Key, "error", err)
			ui.SetFlashError(w, fmt.Sprintf("Invalid value for %s: %v", def.Key, err))
			http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
			return
		}
		h.audit(r, "settings.updated", "setting", def.Key, newVal)
		changed++
	}

	if changed > 0 {
		ui.SetFlash(w, fmt.Sprintf("Updated %d setting(s).", changed))
	} else {
		ui.SetFlash(w, "No settings changed.")
	}
	http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
}

// ResetSetting handles POST /admin/settings/{key}/reset — revert to env default.
func (h *adminHandler) ResetSetting(w http.ResponseWriter, r *http.Request) {

	key := r.PathValue("key")
	if err := h.deps.Settings.Delete(r.Context(), key); err != nil {
		slog.Error("failed to reset setting", "key", key, "error", err)
		ui.SetFlashError(w, fmt.Sprintf("Failed to reset %s.", key))
		http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
		return
	}

	h.audit(r, "settings.reset", "setting", key, "reverted to default")

	ui.SetFlash(w, fmt.Sprintf("Reset %s to default.", key))
	http.Redirect(w, r, "/admin/settings", http.StatusSeeOther)
}

// --- Setting detail page ---

type adminSettingDetailContent struct {
	Def         config.SettingDef
	Value       string
	Source      string // "db", "env", or "default"
	IsOverride  bool
}

// ShowSettingDetail handles GET /admin/settings/{key} — setting documentation page.
func (h *adminHandler) ShowSettingDetail(w http.ResponseWriter, r *http.Request) {

	key := r.PathValue("key")
	def := config.SettingDefByKey(key)
	if def == nil {
		http.NotFound(w, r)
		return
	}

	ctx := r.Context()

	// Determine current value and source.
	snap := h.deps.Settings.Load()
	runtimeValues := snap.ValueMap()
	envValues := h.deps.Config.ValueMap()

	value := envValues[def.Key]
	source := "default"
	if v, ok := runtimeValues[def.Key]; ok {
		value = v
		source = "env"
	}

	overrides, err := h.deps.Settings.Overrides(ctx)
	if err != nil {
		slog.Error("failed to load setting overrides", "error", err)
		overrides = map[string]string{}
	}

	isOverride := false
	if _, ok := overrides[def.Key]; ok {
		source = "db"
		isOverride = true
	}

	content := &adminSettingDetailContent{
		Def:        *def,
		Value:      value,
		Source:     source,
		IsOverride: isOverride,
	}

	h.deps.Renderer.Render(w, "admin_setting_detail", h.deps.adminPageData(w, r, def.Label+" — Settings", "settings", content))
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
