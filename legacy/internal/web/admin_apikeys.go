package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"rstash/internal/db"
	"rstash/internal/model"
	"rstash/internal/ui"
)

type apiKeysHandler struct {
	deps *UIDeps
}

// APIKeysHandler returns handler methods for the admin API Keys page.
func APIKeysHandler(deps *UIDeps) *apiKeysHandler {
	return &apiKeysHandler{deps: deps}
}

type apiKeysListContent struct {
	Keys []*apiKeyRow
}

type apiKeyRow struct {
	ID           int64
	Name         string
	Description  string
	KeyPrefix    string
	RateLimitRPM int
	LastUsedAt   string
	CreatedAt    string
}

type apiKeyFormContent struct {
	Key          *model.APIKey
	Error        string
	RawKey       string // shown once after create/regenerate
}

// ShowList renders GET /admin/api-keys.
func (h *apiKeysHandler) ShowList(w http.ResponseWriter, r *http.Request) {
	keys, err := h.deps.Repo.ListAPIKeys(r.Context())
	if err != nil {
		slog.Error("list api keys", "error", err)
		h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to load API keys.")
		return
	}

	rows := make([]*apiKeyRow, 0, len(keys))
	for _, k := range keys {
		row := &apiKeyRow{
			ID:           k.ID,
			Name:         k.Name,
			Description:  k.Description,
			KeyPrefix:    k.KeyPrefix,
			RateLimitRPM: k.RateLimitRPM,
			CreatedAt:    k.CreatedAt.Format("2006-01-02 15:04"),
		}
		if k.LastUsedAt != nil {
			row.LastUsedAt = k.LastUsedAt.Format("2006-01-02 15:04")
		}
		rows = append(rows, row)
	}

	h.deps.Renderer.Render(w, "admin_apikeys", h.deps.adminPageData(w, r, "API Keys — Admin", "api-keys", &apiKeysListContent{Keys: rows}))
}

// ShowNew renders GET /admin/api-keys/new.
func (h *apiKeysHandler) ShowNew(w http.ResponseWriter, r *http.Request) {
	h.deps.Renderer.Render(w, "admin_apikey_form", h.deps.adminPageData(w, r, "New API Key — Admin", "api-keys", &apiKeyFormContent{
		Key: &model.APIKey{RateLimitRPM: 60},
	}))
}

// DoCreate handles POST /admin/api-keys.
func (h *apiKeysHandler) DoCreate(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	rpm, _ := strconv.Atoi(r.FormValue("rate_limit_rpm"))

	renderForm := func(msg string, key *model.APIKey) {
		h.deps.Renderer.Render(w, "admin_apikey_form", h.deps.adminPageData(w, r, "New API Key — Admin", "api-keys", &apiKeyFormContent{
			Key:   key,
			Error: msg,
		}))
	}

	if name == "" {
		renderForm("Name is required.", &model.APIKey{Name: name, Description: description, RateLimitRPM: rpm})
		return
	}
	if rpm < 0 {
		rpm = 60
	}

	raw, err := db.GenerateAPIKey()
	if err != nil {
		slog.Error("generate api key", "error", err)
		renderForm("Failed to generate key.", &model.APIKey{Name: name, Description: description, RateLimitRPM: rpm})
		return
	}
	hash, err := db.HashAPIKey(raw)
	if err != nil {
		slog.Error("hash api key", "error", err)
		renderForm("Failed to hash key.", &model.APIKey{Name: name, Description: description, RateLimitRPM: rpm})
		return
	}

	key := &model.APIKey{
		Name:         name,
		Description:  description,
		KeyHash:      hash,
		KeyPrefix:    db.APIKeyPrefix(raw),
		RateLimitRPM: rpm,
	}
	if err := h.deps.Repo.CreateAPIKey(r.Context(), key); err != nil {
		slog.Error("create api key", "error", err)
		renderForm("Failed to save key.", key)
		return
	}

	actor := CurrentUser(r)
	actorID := int64(0)
	if actor != nil {
		actorID = actor.ID
	}
	h.deps.Repo.Audit(r.Context(), actorID, "admin.apikey.created", "api_key", fmt.Sprintf("%d", key.ID), name)

	// Render the form once more with the raw key shown — copy-warning pattern.
	h.deps.Renderer.Render(w, "admin_apikey_created", h.deps.adminPageData(w, r, "API Key Created — Admin", "api-keys", &apiKeyFormContent{
		Key:    key,
		RawKey: raw,
	}))
}

// ShowEdit renders GET /admin/api-keys/{id}.
func (h *apiKeysHandler) ShowEdit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	key, err := h.deps.Repo.GetAPIKey(r.Context(), id)
	if err != nil || key == nil {
		http.NotFound(w, r)
		return
	}

	h.deps.Renderer.Render(w, "admin_apikey_edit", h.deps.adminPageData(w, r, "Edit API Key — Admin", "api-keys", &apiKeyFormContent{
		Key: key,
	}))
}

// DoUpdate handles POST /admin/api-keys/{id}.
func (h *apiKeysHandler) DoUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	key, err := h.deps.Repo.GetAPIKey(r.Context(), id)
	if err != nil || key == nil {
		http.NotFound(w, r)
		return
	}

	key.Name = strings.TrimSpace(r.FormValue("name"))
	key.Description = strings.TrimSpace(r.FormValue("description"))
	if rpm, err := strconv.Atoi(r.FormValue("rate_limit_rpm")); err == nil && rpm >= 0 {
		key.RateLimitRPM = rpm
	}
	if key.Name == "" {
		h.deps.Renderer.Render(w, "admin_apikey_edit", h.deps.adminPageData(w, r, "Edit API Key — Admin", "api-keys", &apiKeyFormContent{
			Key:   key,
			Error: "Name is required.",
		}))
		return
	}

	if r.FormValue("regenerate") == "on" {
		raw, err := db.GenerateAPIKey()
		if err != nil {
			slog.Error("generate api key", "error", err)
			h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to regenerate key.")
			return
		}
		hash, err := db.HashAPIKey(raw)
		if err != nil {
			slog.Error("hash api key", "error", err)
			h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to regenerate key.")
			return
		}
		key.KeyHash = hash
		key.KeyPrefix = db.APIKeyPrefix(raw)
		if err := h.deps.Repo.UpdateAPIKey(r.Context(), key); err != nil {
			slog.Error("update api key", "error", err)
			h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to save key.")
			return
		}
		actor := CurrentUser(r)
		actorID := int64(0)
		if actor != nil {
			actorID = actor.ID
		}
		h.deps.Repo.Audit(r.Context(), actorID, "admin.apikey.regenerated", "api_key", fmt.Sprintf("%d", key.ID), key.Name)

		h.deps.Renderer.Render(w, "admin_apikey_created", h.deps.adminPageData(w, r, "API Key Regenerated — Admin", "api-keys", &apiKeyFormContent{
			Key:    key,
			RawKey: raw,
		}))
		return
	}

	if err := h.deps.Repo.UpdateAPIKey(r.Context(), key); err != nil {
		slog.Error("update api key", "error", err)
		h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to save key.")
		return
	}
	actor := CurrentUser(r)
	actorID := int64(0)
	if actor != nil {
		actorID = actor.ID
	}
	h.deps.Repo.Audit(r.Context(), actorID, "admin.apikey.updated", "api_key", fmt.Sprintf("%d", key.ID), key.Name)
	ui.SetFlash(w, "API key updated.")
	http.Redirect(w, r, "/admin/api-keys", http.StatusSeeOther)
}

// DoDelete handles POST /admin/api-keys/{id}/delete.
func (h *apiKeysHandler) DoDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	key, err := h.deps.Repo.GetAPIKey(r.Context(), id)
	if err != nil || key == nil {
		http.NotFound(w, r)
		return
	}
	if err := h.deps.Repo.DeleteAPIKey(r.Context(), id); err != nil {
		slog.Error("delete api key", "error", err)
		h.deps.renderError(w, r, http.StatusInternalServerError, "Internal Error", "Failed to delete key.")
		return
	}
	actor := CurrentUser(r)
	actorID := int64(0)
	if actor != nil {
		actorID = actor.ID
	}
	h.deps.Repo.Audit(r.Context(), actorID, "admin.apikey.deleted", "api_key", fmt.Sprintf("%d", id), key.Name)
	ui.SetFlash(w, "API key deleted.")
	http.Redirect(w, r, "/admin/api-keys", http.StatusSeeOther)
}
