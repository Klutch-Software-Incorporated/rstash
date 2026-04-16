package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"rstash/internal/auth"
	"rstash/internal/db"
	"rstash/internal/model"
)

// meResponse is the OIDC-shaped identity payload returned by GET /api/admin/me.
type meResponse struct {
	Sub                 string  `json:"sub"`
	PreferredUsername   string  `json:"preferred_username"`
	Email               *string `json:"email,omitempty"`
	EmailVerified       bool    `json:"email_verified"`
	StorageQuotaBytes   int64   `json:"storage_quota_bytes"`
	StorageUsedBytes    int64   `json:"storage_used_bytes"`
	EgressQuotaBytes    int64   `json:"egress_quota_bytes,omitempty"`
	EgressUsedBytes     int64   `json:"egress_used_bytes,omitempty"`
	AccountState        string  `json:"account_state"`
	ExternallyManaged   bool    `json:"externally_managed,omitempty"`
	CreatedAt           string  `json:"created_at"`
}

// getMe returns the user identified by the incoming rstash_session cookie,
// or 401 if the session is missing/invalid.
//
// Intended for companion apps on subdomains (e.g. billing.example.com) that
// share the session cookie via the cookie_domain setting. The client forwards
// the incoming session cookie and authenticates to rstash with its admin API
// key; rstash validates the session and returns the identity to the client,
// which then drops its own session cookie and renders the page.
func (d *AdminDeps) getMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	cookie, err := r.Cookie(auth.SessionCookieName)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "no session cookie")
		return
	}

	sess, err := d.Repo.GetSessionByToken(ctx, cookie.Value)
	if err != nil {
		slog.Error("admin API /me: get session", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load session")
		return
	}
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "session expired or invalid")
		return
	}

	user, err := d.Repo.GetUserByID(ctx, sess.UserID)
	if err != nil {
		slog.Error("admin API /me: get user", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to load user")
		return
	}
	if user == nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	stats, _ := d.Repo.GetUserStorageStats(ctx, user.ID)
	resp := meResponse{
		Sub:               strconv.FormatInt(user.ID, 10),
		PreferredUsername: user.Username,
		Email:             user.Email,
		EmailVerified:     user.EmailVerified,
		StorageQuotaBytes: user.StorageQuota,
		AccountState:      accountState(user),
		ExternallyManaged: user.ExternallyManaged,
		CreatedAt:         user.CreatedAt.Format(time.RFC3339),
	}
	if stats != nil {
		resp.StorageUsedBytes = stats.TotalBytes
	}

	// Egress fields are omitted from the JSON when both are zero (see the
	// omitempty tags above), which effectively hides them when egress_mode=off.
	if eg, _ := d.Repo.GetEgressUsage(ctx, user.ID, db.CurrentPeriod()); eg != nil {
		resp.EgressUsedBytes = eg.BytesOut
	}
	resp.EgressQuotaBytes = user.EgressQuota

	writeJSON(w, http.StatusOK, map[string]any{"data": resp})
}

// accountState derives the display state for the /me response.
// Extended in Phase 3 to return "unclaimed" for externally-provisioned
// users who have not yet set a password.
func accountState(user *model.User) string {
	if user.Disabled {
		return "disabled"
	}
	return "active"
}
