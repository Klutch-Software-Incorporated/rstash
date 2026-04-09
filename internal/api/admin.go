package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"rstash/internal/db"
	"rstash/internal/storage"
)

// AdminDeps holds dependencies for the admin API handlers.
type AdminDeps struct {
	Repo    *db.Repository
	Storage *storage.Service
	APIKey  string
}

// AdminRoutes returns an http.Handler that serves the admin JSON API.
// All routes are prefixed with /api/admin/ and protected by API key auth.
func AdminRoutes(deps *AdminDeps) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/admin/users", deps.listUsers)
	mux.HandleFunc("GET /api/admin/users/{username}", deps.getUser)
	mux.HandleFunc("POST /api/admin/users", deps.createUser)
	mux.HandleFunc("PUT /api/admin/users/{username}/quota", deps.setUserQuota)
	mux.HandleFunc("PUT /api/admin/users/{username}/disable", deps.setUserDisabled)
	mux.HandleFunc("DELETE /api/admin/users/{username}", deps.deleteUser)
	mux.HandleFunc("GET /api/admin/stats", deps.getStats)

	return RequireAPIKey(deps.APIKey)(jsonContentType(mux))
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

func (d *AdminDeps) createUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Email    string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	user, err := d.Repo.CreateUser(r.Context(), req.Username, req.Password, req.Email, false, true)
	if err != nil {
		slog.Error("admin API: create user", "error", err)
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	d.Repo.Audit(r.Context(), 0, "admin_api.user.create", "user", user.Username, "")

	writeJSON(w, http.StatusCreated, map[string]any{
		"data": apiUser{
			Username:  user.Username,
			Email:     user.Email,
			Approved:  user.Approved,
			CreatedAt: user.CreatedAt.Format(time.RFC3339),
		},
	})
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

func (d *AdminDeps) setUserDisabled(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")

	var req struct {
		Disabled bool `json:"disabled"`
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

	if err := d.Repo.UpdateUserDisabled(r.Context(), user.ID, req.Disabled); err != nil {
		slog.Error("admin API: set disabled", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	d.Repo.Audit(r.Context(), 0, "admin_api.user.disable", "user", username, "")

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
