package web

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"rstash/internal/auth"
	"rstash/internal/ui"
)

// accountRightsHandler serves the user-facing GDPR surfaces: self-service
// account deletion, data export, and the activity log.
type accountRightsHandler struct {
	deps *UIDeps
}

// AccountRightsHandler constructs the handler bundle.
func AccountRightsHandler(deps *UIDeps) *accountRightsHandler {
	return &accountRightsHandler{deps: deps}
}

// --- Delete ---

type accountDeleteContent struct {
	Username           string
	ExternallyManaged  bool
	ExternalAccountURL string
	Error              string
}

func (h *accountRightsHandler) ShowDelete(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)
	snap := h.deps.Settings.Load()
	h.deps.Renderer.Render(w, "account_delete", h.deps.pageData(w, r, "Delete Account — rstash", &accountDeleteContent{
		Username:           user.Username,
		ExternallyManaged:  user.ExternallyManaged,
		ExternalAccountURL: snap.ExternalAccountURL,
	}))
}

func (h *accountRightsHandler) DoDelete(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)
	snap := h.deps.Settings.Load()

	// Externally-managed users: bounce them to the external portal.
	if user.ExternallyManaged && snap.ExternalAccountURL != "" {
		http.Redirect(w, r, snap.ExternalAccountURL, http.StatusSeeOther)
		return
	}

	// Require password confirmation on local accounts.
	password := r.FormValue("password")
	if !h.deps.Auth.CheckPassword(user, password) {
		h.deps.Renderer.Render(w, "account_delete", h.deps.pageData(w, r, "Delete Account — rstash", &accountDeleteContent{
			Username:          user.Username,
			ExternallyManaged: user.ExternallyManaged,
			Error:             "Incorrect password. Deletion cancelled.",
		}))
		return
	}

	if err := h.deps.Repo.AnonymizeAuditEntriesForUser(r.Context(), user.ID, user.Username); err != nil {
		slog.Error("anonymize audit on delete", "error", err)
	}
	if err := h.deps.Auth.DeleteUser(r.Context(), user.ID); err != nil {
		slog.Error("self-delete user", "error", err)
		h.deps.Renderer.Render(w, "account_delete", h.deps.pageData(w, r, "Delete Account — rstash", &accountDeleteContent{
			Username: user.Username,
			Error:    "Failed to delete account. Try again or contact an administrator.",
		}))
		return
	}

	if h.deps.Webhooks != nil {
		h.deps.Webhooks.Emit(r.Context(), "user.deleted", map[string]any{"username": user.Username})
	}

	auth.ClearSessionCookie(w, snap.CookieDomain, h.deps.SecureCookies)
	ui.SetFlash(w, "Your account has been deleted.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- Export ---

type exportEnvelope struct {
	Profile       exportProfile       `json:"profile"`
	Sessions      []exportSession     `json:"sessions"`
	OAuthClients  []exportOAuthClient `json:"oauth_clients"`
	AuditEntries  []exportAuditEntry  `json:"audit_entries"`
	FileManifest  []exportFile        `json:"file_manifest"`
	ExportedAt    string              `json:"exported_at"`
}

type exportProfile struct {
	Username       string  `json:"username"`
	Email          *string `json:"email,omitempty"`
	EmailVerified  bool    `json:"email_verified"`
	CreatedAt      string  `json:"created_at"`
	StorageQuota   int64   `json:"storage_quota_bytes"`
	BandwidthQuota int64   `json:"bandwidth_quota_bytes"`
}

type exportSession struct {
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
	IP        string `json:"ip,omitempty"`
}

type exportOAuthClient struct {
	ClientID  string `json:"client_id"`
	Scopes    string `json:"scopes"`
	CreatedAt string `json:"created_at"`
}

type exportAuditEntry struct {
	Action     string `json:"action"`
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
	Details    string `json:"details,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type exportFile struct {
	Path          string `json:"path"`
	ContentType   string `json:"content_type"`
	ContentLength int64  `json:"content_length"`
	ETag          string `json:"etag"`
	UpdatedAt     string `json:"updated_at"`
}

func (h *accountRightsHandler) DoExport(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)
	ctx := r.Context()

	env := exportEnvelope{
		Profile: exportProfile{
			Username:       user.Username,
			Email:          user.Email,
			EmailVerified:  user.EmailVerified,
			CreatedAt:      user.CreatedAt.Format(time.RFC3339),
			StorageQuota:   user.StorageQuota,
			BandwidthQuota: user.BandwidthQuota,
		},
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if sessions, err := h.deps.Auth.ListUserSessions(ctx, user.ID); err == nil {
		for _, s := range sessions {
			ip := ""
			if s.IP != nil {
				ip = *s.IP
			}
			env.Sessions = append(env.Sessions, exportSession{
				CreatedAt: s.CreatedAt.Format(time.RFC3339),
				ExpiresAt: s.ExpiresAt.Format(time.RFC3339),
				IP:        ip,
			})
		}
	}

	if tokens, err := h.deps.Repo.ListOAuthTokensByUserID(ctx, user.ID); err == nil {
		for _, t := range tokens {
			env.OAuthClients = append(env.OAuthClients, exportOAuthClient{
				ClientID:  t.ClientID,
				Scopes:    t.Scopes,
				CreatedAt: t.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	if rows, err := h.deps.Repo.ListAuditEntriesByActor(ctx, user.ID, 1000); err == nil {
		for _, a := range rows {
			env.AuditEntries = append(env.AuditEntries, exportAuditEntry{
				Action:     a.Action,
				TargetType: a.TargetType,
				TargetID:   a.TargetID,
				Details:    a.Details,
				CreatedAt:  a.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	if nodes, err := h.deps.Repo.ListUserNodes(ctx, user.ID); err == nil {
		for _, n := range nodes {
			env.FileManifest = append(env.FileManifest, exportFile{
				Path:          n.Path,
				ContentType:   n.ContentType,
				ContentLength: n.ContentLength,
				ETag:          n.ETag,
				UpdatedAt:     n.UpdatedAt.Format(time.RFC3339),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"rstash-export-%s-%s.json\"", user.Username, time.Now().UTC().Format("2006-01-02")))
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(env); err != nil {
		slog.Error("encode export", "error", err)
	}
}

// --- Activity log ---

type activityContent struct {
	Entries []activityRow
}

type activityRow struct {
	Action     string
	TargetType string
	TargetID   string
	Details    string
	CreatedAt  string
}

func (h *accountRightsHandler) ShowActivity(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)

	rows, err := h.deps.Repo.ListAuditEntriesByActor(r.Context(), user.ID, 200)
	if err != nil {
		slog.Error("list activity", "error", err)
		h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to load activity.")
		return
	}

	entries := make([]activityRow, 0, len(rows))
	for _, a := range rows {
		entries = append(entries, activityRow{
			Action:     a.Action,
			TargetType: a.TargetType,
			TargetID:   a.TargetID,
			Details:    a.Details,
			CreatedAt:  a.CreatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	h.deps.Renderer.Render(w, "account_activity", h.deps.pageData(w, r, "Activity — rstash", &activityContent{Entries: entries}))
}

// --- OAuth app management ---

type oauthAppsContent struct {
	Apps []oauthAppRow
}

type oauthAppRow struct {
	ClientID    string
	Scopes      string
	CreatedAt   string
	TokenPrefix string
	TokenFull   string // for the revoke form
}

func (h *accountRightsHandler) ShowOAuthApps(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)
	tokens, err := h.deps.Repo.ListOAuthTokensByUserID(r.Context(), user.ID)
	if err != nil {
		slog.Error("list oauth tokens", "error", err)
		h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to load OAuth apps.")
		return
	}
	apps := make([]oauthAppRow, 0, len(tokens))
	for _, t := range tokens {
		apps = append(apps, oauthAppRow{
			ClientID:    t.ClientID,
			Scopes:      strings.ReplaceAll(t.Scopes, " ", ", "),
			CreatedAt:   t.CreatedAt.Format("2006-01-02 15:04"),
			TokenPrefix: t.Token[:8] + "...",
			TokenFull:   t.Token,
		})
	}
	h.deps.Renderer.Render(w, "account_apps", h.deps.pageData(w, r, "OAuth Apps — rstash", &oauthAppsContent{Apps: apps}))
}

func (h *accountRightsHandler) RevokeOAuthApp(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)
	clientID := r.FormValue("client_id")
	if clientID == "" {
		http.Error(w, "missing client_id", http.StatusBadRequest)
		return
	}
	n, err := h.deps.Repo.DeleteUserOAuthTokensForClient(r.Context(), user.ID, clientID)
	if err != nil {
		slog.Error("revoke oauth app", "error", err)
		ui.SetFlashError(w, "Failed to revoke.")
		http.Redirect(w, r, "/account/apps", http.StatusSeeOther)
		return
	}
	h.deps.Repo.Audit(r.Context(), user.ID, "user.oauth_revoked", "oauth_client", clientID, fmt.Sprintf("%d tokens", n))
	ui.SetFlash(w, "OAuth app access revoked.")
	http.Redirect(w, r, "/account/apps", http.StatusSeeOther)
}
