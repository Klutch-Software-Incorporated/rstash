package api

import (
	cryptoRand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"rstash/internal/db"
	"rstash/internal/storage"
	"rstash/internal/webhooks"
)

// AdminDeps holds dependencies for the admin API handlers.
type AdminDeps struct {
	Repo    *db.Repository
	Storage *storage.Service
	// BaseURL is used to construct claim URLs when provisioning users.
	BaseURL string
	// RateLimiter enforces per-APIKey RateLimitRPM. Nil disables rate limiting
	// (e.g. in tests).
	RateLimiter *APIKeyRateLimiter
	// Webhooks emits state-change events to registered subscribers. Nil disables.
	Webhooks *webhooks.Emitter
}

// AdminRoutes returns an http.Handler that serves the admin JSON API.
// All routes are prefixed with /api/admin/ and protected by API key auth.
func AdminRoutes(deps *AdminDeps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/admin/me", deps.getMe)
	mux.HandleFunc("GET /api/admin/users", deps.listUsers)
	mux.HandleFunc("GET /api/admin/users/{username}", deps.getUser)
	mux.HandleFunc("POST /api/admin/users", deps.createUser)
	mux.HandleFunc("PUT /api/admin/users/{username}/quota", deps.setUserQuota)
	mux.HandleFunc("PUT /api/admin/users/{username}/email", deps.setUserEmail)
	mux.HandleFunc("POST /api/admin/users/{username}/disable", deps.disableUser)
	mux.HandleFunc("POST /api/admin/users/{username}/enable", deps.enableUser)
	mux.HandleFunc("DELETE /api/admin/users/{username}", deps.deleteUser)
	mux.HandleFunc("GET /api/admin/users/{username}/usage", deps.getUserUsage)
	mux.HandleFunc("PUT /api/admin/users/{username}/bandwidth_quota", deps.setUserBandwidthQuota)
	mux.HandleFunc("POST /api/admin/users/{username}/reset_bandwidth", deps.resetUserBandwidth)
	mux.HandleFunc("GET /api/admin/stats", deps.getStats)

	return RequireAPIKey(deps.Repo, deps.RateLimiter)(jsonContentType(mux))
}

// jsonContentType sets Content-Type: application/json on all responses.
func jsonContentType(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- Response types ---

type apiUser struct {
	Username     string  `json:"username"`
	Email        *string `json:"email,omitempty"`
	IsAdmin      bool    `json:"is_admin"`
	Disabled     bool    `json:"disabled"`
	Approved     bool    `json:"approved"`
	StorageQuota int64   `json:"storage_quota"`
	StorageUsed  int64   `json:"storage_used"`
	FileCount    int64   `json:"file_count"`
	CreatedAt    string  `json:"created_at"`
	LastLoginAt  *string `json:"last_login_at,omitempty"`
}

type listUsersResponse struct {
	Data  []apiUser `json:"data"`
	Total int       `json:"total"`
}

type statsResponse struct {
	TotalUsers      int64 `json:"total_users"`
	TotalStorageBytes int64 `json:"total_storage_bytes"`
	TotalDocuments  int64 `json:"total_documents"`
}

// --- Handlers ---

func (d *AdminDeps) listUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	users, err := d.Repo.ListUsers(ctx)
	if err != nil {
		slog.Error("admin API: list users", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	data := make([]apiUser, 0, len(users))
	for _, u := range users {
		stats, _ := d.Repo.GetUserStorageStats(ctx, u.ID)
		au := apiUser{
			Username:     u.Username,
			Email:        u.Email,
			IsAdmin:      u.IsAdmin,
			Disabled:     u.Disabled,
			Approved:     u.Approved,
			StorageQuota: u.StorageQuota,
			CreatedAt:    u.CreatedAt.Format(time.RFC3339),
		}
		if stats != nil {
			au.StorageUsed = stats.TotalBytes
			au.FileCount = stats.FileCount
		}
		if u.LastLoginAt != nil {
			t := u.LastLoginAt.Format(time.RFC3339)
			au.LastLoginAt = &t
		}
		data = append(data, au)
	}

	writeJSON(w, http.StatusOK, listUsersResponse{Data: data, Total: len(data)})
}

func (d *AdminDeps) getUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	username := r.PathValue("username")

	user, err := d.Repo.GetUserByUsername(ctx, username)
	if err != nil {
		slog.Error("admin API: get user", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}
	if user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	stats, _ := d.Repo.GetUserStorageStats(ctx, user.ID)
	au := apiUser{
		Username:     user.Username,
		Email:        user.Email,
		IsAdmin:      user.IsAdmin,
		Disabled:     user.Disabled,
		Approved:     user.Approved,
		StorageQuota: user.StorageQuota,
		CreatedAt:    user.CreatedAt.Format(time.RFC3339),
	}
	if stats != nil {
		au.StorageUsed = stats.TotalBytes
		au.FileCount = stats.FileCount
	}
	if user.LastLoginAt != nil {
		t := user.LastLoginAt.Format(time.RFC3339)
		au.LastLoginAt = &t
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": au})
}

// claimTokenLifetime is how long a provisioning claim token remains valid.
const claimTokenLifetime = 24 * time.Hour

func (d *AdminDeps) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username            string `json:"username"`
		Password            string `json:"password"`
		Email               string `json:"email"`
		EmailVerified       bool   `json:"email_verified"`
		Provision           bool   `json:"provision"`
		QuotaBytes          int64  `json:"quota_bytes"`
		BandwidthQuotaBytes int64  `json:"bandwidth_quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if !req.Provision && req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required (or set provision=true to issue a claim token)")
		return
	}

	password := req.Password
	if req.Provision {
		// Unguessable placeholder — password login will fail until claim.
		rand, err := randomHex(32)
		if err != nil {
			slog.Error("admin API: generate placeholder password", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to prepare account")
			return
		}
		password = rand
	}

	user, err := d.Repo.CreateUser(r.Context(), req.Username, password, req.Email, false, true)
	if err != nil {
		slog.Error("admin API: create user", "error", err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	if req.QuotaBytes > 0 {
		if err := d.Repo.UpdateUserQuota(r.Context(), user.ID, req.QuotaBytes); err != nil {
			slog.Error("admin API: set quota on create", "error", err)
		}
		user.StorageQuota = req.QuotaBytes
	}

	if req.BandwidthQuotaBytes > 0 {
		if err := d.Repo.UpdateUserBandwidthQuota(r.Context(), user.ID, req.BandwidthQuotaBytes); err != nil {
			slog.Error("admin API: set bandwidth quota on create", "error", err)
		}
		user.BandwidthQuota = req.BandwidthQuotaBytes
	}

	if req.EmailVerified && user.Email != nil {
		if err := d.Repo.VerifyUserEmail(r.Context(), user.ID); err != nil {
			slog.Error("admin API: verify email on create", "error", err)
		}
		user.EmailVerified = true
	}

	resp := map[string]any{
		"data": apiUser{
			Username:     user.Username,
			Email:        user.Email,
			Approved:     user.Approved,
			StorageQuota: user.StorageQuota,
			CreatedAt:    user.CreatedAt.Format(time.RFC3339),
		},
	}

	if req.Provision {
		// Mark as externally managed and issue a claim token.
		if err := d.Repo.SetExternallyManaged(r.Context(), user.ID, true); err != nil {
			slog.Error("admin API: set externally_managed", "error", err)
		}
		token, err := randomHex(32)
		if err != nil {
			slog.Error("admin API: generate claim token", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to generate claim token")
			return
		}
		expiry := time.Now().UTC().Add(claimTokenLifetime)
		if err := d.Repo.SetPasswordResetToken(r.Context(), user.ID, token, expiry); err != nil {
			slog.Error("admin API: set claim token", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to save claim token")
			return
		}

		claimURL := d.BaseURL + "/claim?token=" + token
		resp["claim_url"] = claimURL
		resp["claim_token_expires_at"] = expiry.Format(time.RFC3339)

		d.Repo.Audit(r.Context(), 0, "admin_api.user.provisioned", "user", user.Username, "")
	} else {
		d.Repo.Audit(r.Context(), 0, "admin_api.user.create", "user", user.Username, "")
	}

	writeJSON(w, http.StatusCreated, resp)
}


// randomHex returns n random bytes encoded as 2n hex chars.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := cryptoRand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (d *AdminDeps) setUserQuota(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")

	var req struct {
		QuotaBytes int64 `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, err := d.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if err := d.Repo.UpdateUserQuota(r.Context(), user.ID, req.QuotaBytes); err != nil {
		slog.Error("admin API: set quota", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update quota")
		return
	}

	d.Repo.Audit(r.Context(), 0, "admin_api.user.quota", "user", username, "")

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (d *AdminDeps) setUserEmail(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")

	var req struct {
		Email    string `json:"email"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	email, err := db.ValidateEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	user, err := d.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	// Ensure the new email isn't already taken by someone else.
	if existing, _ := d.Repo.GetUserByEmail(r.Context(), email); existing != nil && existing.ID != user.ID {
		writeError(w, http.StatusConflict, "email is already in use by another account")
		return
	}

	if err := d.Repo.UpdateUserEmail(r.Context(), user.ID, email); err != nil {
		slog.Error("admin API: update email", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update email")
		return
	}

	if req.Verified {
		if err := d.Repo.VerifyUserEmail(r.Context(), user.ID); err != nil {
			slog.Error("admin API: verify email", "error", err)
		}
	}

	d.Repo.Audit(r.Context(), 0, "admin_api.user.email_updated", "user", username, email)
	if d.Webhooks != nil {
		d.Webhooks.Emit(r.Context(), "user.email_changed", map[string]any{
			"username":   username,
			"new_email":  email,
			"changed_by": "admin_api",
		})
	}
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

// disableUser disables a user account. No request body. Idempotent:
// calling this on an already-disabled user returns 200 and re-emits
// the webhook (so downstream subscribers can be re-synced if needed).
func (d *AdminDeps) disableUser(w http.ResponseWriter, r *http.Request) {
	d.setDisabledState(w, r, true)
}

// enableUser clears the disabled flag on a user account. No request body.
// Idempotent: same behavior as disableUser when already enabled.
func (d *AdminDeps) enableUser(w http.ResponseWriter, r *http.Request) {
	d.setDisabledState(w, r, false)
}

func (d *AdminDeps) setDisabledState(w http.ResponseWriter, r *http.Request, disabled bool) {
	username := r.PathValue("username")

	user, err := d.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if err := d.Repo.UpdateUserDisabled(r.Context(), user.ID, disabled); err != nil {
		slog.Error("admin API: set disabled", "error", err, "disabled", disabled)
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	action := "admin_api.user.enabled"
	event := "user.enabled"
	if disabled {
		action = "admin_api.user.disabled"
		event = "user.disabled"
	}
	d.Repo.Audit(r.Context(), 0, action, "user", username, "")
	if d.Webhooks != nil {
		d.Webhooks.Emit(r.Context(), event, map[string]any{"username": username})
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (d *AdminDeps) deleteUser(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")

	user, err := d.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	if err := d.Repo.DeleteUser(r.Context(), user.ID); err != nil {
		slog.Error("admin API: delete user", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	d.Repo.Audit(r.Context(), 0, "admin_api.user.delete", "user", username, "")
	if d.Webhooks != nil {
		d.Webhooks.Emit(r.Context(), "user.deleted", map[string]any{"username": username})
	}

	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (d *AdminDeps) getUserUsage(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	user, err := d.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	stats, _ := d.Repo.GetUserStorageStats(r.Context(), user.ID)
	period := db.CurrentPeriod()
	bw, _ := d.Repo.GetBandwidthUsage(r.Context(), user.ID, period)

	storageUsed := int64(0)
	if stats != nil {
		storageUsed = stats.TotalBytes
	}
	bandwidthUsed := int64(0)
	if bw != nil {
		bandwidthUsed = bw.BytesOut
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"storage": map[string]any{
				"quota_bytes": user.StorageQuota,
				"used_bytes":  storageUsed,
			},
			"bandwidth": map[string]any{
				"quota_bytes": user.BandwidthQuota,
				"used_bytes":  bandwidthUsed,
				"period":      period,
			},
		},
	})
}

func (d *AdminDeps) setUserBandwidthQuota(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	var req struct {
		QuotaBytes int64 `json:"quota_bytes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	user, err := d.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := d.Repo.UpdateUserBandwidthQuota(r.Context(), user.ID, req.QuotaBytes); err != nil {
		slog.Error("admin API: set bandwidth quota", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update bandwidth quota")
		return
	}
	d.Repo.Audit(r.Context(), 0, "admin_api.user.bandwidth_quota", "user", username, "")
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (d *AdminDeps) resetUserBandwidth(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	user, err := d.Repo.GetUserByUsername(r.Context(), username)
	if err != nil || user == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := d.Repo.ResetBandwidthUsage(r.Context(), user.ID, db.CurrentPeriod()); err != nil {
		slog.Error("admin API: reset bandwidth", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to reset bandwidth")
		return
	}
	d.Repo.Audit(r.Context(), 0, "admin_api.user.reset_bandwidth", "user", username, "")
	writeJSON(w, http.StatusOK, map[string]string{"ok": "true"})
}

func (d *AdminDeps) getStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userCount, err := d.Repo.UserCount(ctx)
	if err != nil {
		slog.Error("admin API: user count", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	totalStorage, err := d.Repo.GetTotalStorageUsed(ctx)
	if err != nil {
		slog.Error("admin API: total storage", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	totalDocs, err := d.Repo.CountDocumentNodes(ctx)
	if err != nil {
		slog.Error("admin API: document count", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}

	writeJSON(w, http.StatusOK, statsResponse{
		TotalUsers:        userCount,
		TotalStorageBytes: totalStorage,
		TotalDocuments:    totalDocs,
	})
}
