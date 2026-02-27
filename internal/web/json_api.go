package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gosilo/api"
	"gosilo/internal/auth"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/model"
)

// Compile-time check: jsonApiHandler must implement the generated ServerInterface.
var _ api.ServerInterface = (*jsonApiHandler)(nil)

type jsonApiHandler struct {
	deps *UIDeps
}

// JSONApiHandler returns handler methods for the JSON management API.
func JSONApiHandler(deps *UIDeps) *jsonApiHandler {
	return &jsonApiHandler{deps: deps}
}

// --- Response helpers ---

func jsonOK(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "payload": payload})
}

func jsonErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": msg})
}

// --- Login/Logout ---

// Login handles POST /json/login — authenticates and returns a session token.
// The json_api setting check is handled by the global middleware.
func (h *jsonApiHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, err := h.deps.Auth.Authenticate(r.Context(), req.Username, req.Password)
	if err != nil {
		jsonErr(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if !user.IsAdmin {
		jsonErr(w, http.StatusForbidden, "admin access required")
		return
	}

	sess, err := h.deps.Auth.CreateSession(r.Context(), user.ID)
	if err != nil {
		slog.Error("json api: failed to create session", "error", err)
		jsonErr(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	db.Audit(r.Context(), h.deps.DB, user.ID, "json_api.login", "user", fmt.Sprintf("%d", user.ID), user.Username)

	jsonOK(w, map[string]string{"authToken": sess.Token})
}

// Logout handles POST /json/logout — destroys the current session.
// Auth is handled by the global middleware; this just extracts the token to destroy it.
func (h *jsonApiHandler) Logout(w http.ResponseWriter, r *http.Request) {
	token := ""
	if cookie, err := r.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" {
		token = cookie.Value
	}
	if token == "" {
		if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
			token = strings.TrimPrefix(ah, "Bearer ")
		}
	}

	if token != "" {
		h.deps.Auth.DestroySession(r.Context(), token)
	}

	jsonOK(w, map[string]string{"status": "logged out"})
}

// --- User endpoints ---

// UserList handles GET /json/user/list.
func (h *jsonApiHandler) UserList(w http.ResponseWriter, r *http.Request) {
	users, err := h.deps.Auth.ListUsers(r.Context())
	if err != nil {
		slog.Error("json api: failed to list users", "error", err)
		jsonErr(w, http.StatusInternalServerError, "failed to list users")
		return
	}

	type userJSON struct {
		ID                int64   `json:"id"`
		Username          string  `json:"username"`
		IsAdmin           bool    `json:"is_admin"`
		Disabled          bool    `json:"disabled"`
		StorageQuota      int64   `json:"storage_quota"`
		CreatedAt         string  `json:"created_at"`
		LastLoginAt       *string `json:"last_login_at"`
		TOSAcceptedAt     *string `json:"tos_accepted_at"`
		PrivacyAcceptedAt *string `json:"privacy_accepted_at"`
	}

	result := make([]userJSON, len(users))
	for i, u := range users {
		result[i] = userJSON{
			ID:                u.ID,
			Username:          u.Username,
			IsAdmin:           u.IsAdmin,
			Disabled:          u.Disabled,
			StorageQuota:      u.StorageQuota,
			CreatedAt:         u.CreatedAt,
			LastLoginAt:       u.LastLoginAt,
			TOSAcceptedAt:     u.TOSAcceptedAt,
			PrivacyAcceptedAt: u.PrivacyAcceptedAt,
		}
	}

	jsonOK(w, map[string]any{"users": result})
}

// UserAdd handles POST /json/user/add.
func (h *jsonApiHandler) UserAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Username == "" || req.Password == "" {
		jsonErr(w, http.StatusBadRequest, "username and password are required")
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		jsonErr(w, http.StatusBadRequest, msg)
		return
	}

	newUser, err := h.deps.Auth.CreateUser(r.Context(), req.Username, req.Password, req.IsAdmin)
	if err != nil {
		jsonErr(w, http.StatusConflict, "failed to create user (username may already exist)")
		return
	}

	// Auto-accept TOS/Privacy for admin-created users.
	_ = db.AcceptTOS(r.Context(), h.deps.DB, newUser.ID)
	_ = db.AcceptPrivacy(r.Context(), h.deps.DB, newUser.ID)

	actor := currentUserFromContext(r)
	db.Audit(r.Context(), h.deps.DB, actor.ID, "user.created", "user", fmt.Sprintf("%d", newUser.ID), req.Username)

	jsonOK(w, map[string]any{
		"id":       newUser.ID,
		"username": newUser.Username,
		"is_admin": newUser.IsAdmin,
	})
}

// UserDelete handles POST /json/user/delete.
func (h *jsonApiHandler) UserDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, err := h.deps.Auth.GetUserByUsername(r.Context(), req.Username)
	if err != nil || user == nil {
		jsonErr(w, http.StatusNotFound, "user not found")
		return
	}

	actor := currentUserFromContext(r)
	if user.ID == actor.ID {
		jsonErr(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	if err := h.deps.Auth.DeleteUser(r.Context(), user.ID); err != nil {
		slog.Error("json api: failed to delete user", "error", err)
		jsonErr(w, http.StatusInternalServerError, "failed to delete user")
		return
	}

	db.Audit(r.Context(), h.deps.DB, actor.ID, "user.deleted", "user", fmt.Sprintf("%d", user.ID), user.Username)
	jsonOK(w, map[string]string{"status": "deleted"})
}

// UserPasswd handles POST /json/user/passwd.
func (h *jsonApiHandler) UserPasswd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Password == "" {
		jsonErr(w, http.StatusBadRequest, "password is required")
		return
	}
	if msg := validatePassword(req.Password); msg != "" {
		jsonErr(w, http.StatusBadRequest, msg)
		return
	}

	user, err := h.deps.Auth.GetUserByUsername(r.Context(), req.Username)
	if err != nil || user == nil {
		jsonErr(w, http.StatusNotFound, "user not found")
		return
	}

	if err := h.deps.Auth.UpdatePassword(r.Context(), user.ID, req.Password); err != nil {
		slog.Error("json api: failed to update password", "error", err)
		jsonErr(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	actor := currentUserFromContext(r)
	db.Audit(r.Context(), h.deps.DB, actor.ID, "user.password_changed", "user", fmt.Sprintf("%d", user.ID), user.Username)
	jsonOK(w, map[string]string{"status": "password updated"})
}

// UserPromote handles POST /json/user/promote — toggles admin status.
func (h *jsonApiHandler) UserPromote(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, err := h.deps.Auth.GetUserByUsername(r.Context(), req.Username)
	if err != nil || user == nil {
		jsonErr(w, http.StatusNotFound, "user not found")
		return
	}

	actor := currentUserFromContext(r)
	if user.ID == actor.ID {
		jsonErr(w, http.StatusBadRequest, "cannot change your own admin status")
		return
	}

	newAdmin := !user.IsAdmin
	if err := h.deps.Auth.ToggleAdmin(r.Context(), user.ID, newAdmin); err != nil {
		slog.Error("json api: failed to toggle admin", "error", err)
		jsonErr(w, http.StatusInternalServerError, "failed to toggle admin")
		return
	}

	db.Audit(r.Context(), h.deps.DB, actor.ID, "user.admin_toggled", "user", fmt.Sprintf("%d", user.ID), fmt.Sprintf("admin=%v", newAdmin))
	jsonOK(w, map[string]any{"username": user.Username, "is_admin": newAdmin})
}

// UserDisable handles POST /json/user/disable — toggles disabled status.
func (h *jsonApiHandler) UserDisable(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	user, err := h.deps.Auth.GetUserByUsername(r.Context(), req.Username)
	if err != nil || user == nil {
		jsonErr(w, http.StatusNotFound, "user not found")
		return
	}

	actor := currentUserFromContext(r)
	if user.ID == actor.ID {
		jsonErr(w, http.StatusBadRequest, "cannot disable your own account")
		return
	}

	newDisabled := !user.Disabled
	if err := h.deps.Auth.SetDisabled(r.Context(), user.ID, newDisabled); err != nil {
		slog.Error("json api: failed to toggle disabled", "error", err)
		jsonErr(w, http.StatusInternalServerError, "failed to toggle disabled")
		return
	}

	if newDisabled {
		h.deps.Auth.TerminateAllSessions(r.Context(), user.ID)
		_ = db.DeleteOAuthTokensByUserID(r.Context(), h.deps.DB, user.ID)
		_ = db.DeleteRefreshTokensByUserID(r.Context(), h.deps.DB, user.ID)
		db.Audit(r.Context(), h.deps.DB, actor.ID, "user.disabled", "user", fmt.Sprintf("%d", user.ID), user.Username)
	} else {
		db.Audit(r.Context(), h.deps.DB, actor.ID, "user.enabled", "user", fmt.Sprintf("%d", user.ID), user.Username)
	}

	jsonOK(w, map[string]any{"username": user.Username, "disabled": newDisabled})
}

// --- Config endpoints ---

// ConfigList handles GET /json/config/list.
func (h *jsonApiHandler) ConfigList(w http.ResponseWriter, r *http.Request) {
	snap := h.deps.Settings.Load()
	runtimeValues := snap.ValueMap()
	envValues := h.deps.Config.ValueMap()

	overrides, err := h.deps.Settings.Overrides(r.Context())
	if err != nil {
		overrides = map[string]string{}
	}

	type settingJSON struct {
		Key             string   `json:"key"`
		Value           string   `json:"value"`
		Default         string   `json:"default"`
		Source          string   `json:"source"` // "default", "env", "db"
		RuntimeEditable bool     `json:"runtime_editable"`
		ValidValues     []string `json:"valid_values,omitempty"`
	}

	var settings []settingJSON
	for _, def := range config.SettingDefs() {
		value := envValues[def.Key]
		source := "default"
		if def.EnvVar != "" {
			source = "env"
		}
		if v, ok := runtimeValues[def.Key]; ok {
			value = v
		}
		if _, ok := overrides[def.Key]; ok {
			source = "db"
		}

		settings = append(settings, settingJSON{
			Key:             def.Key,
			Value:           value,
			Default:         def.Default,
			Source:          source,
			RuntimeEditable: def.RuntimeEditable,
			ValidValues:     def.ValidValues,
		})
	}

	jsonOK(w, map[string]any{"settings": settings})
}

// ConfigGet handles GET /json/config/get?key=...
func (h *jsonApiHandler) ConfigGet(w http.ResponseWriter, r *http.Request, params api.ConfigGetParams) {
	key := params.Key
	def := config.SettingDefByKey(key)
	if def == nil {
		jsonErr(w, http.StatusNotFound, "unknown setting")
		return
	}

	snap := h.deps.Settings.Load()
	runtimeValues := snap.ValueMap()
	envValues := h.deps.Config.ValueMap()

	value := envValues[def.Key]
	source := "default"
	if def.EnvVar != "" {
		source = "env"
	}
	if v, ok := runtimeValues[def.Key]; ok {
		value = v
	}

	overrides, err := h.deps.Settings.Overrides(r.Context())
	if err != nil {
		overrides = map[string]string{}
	}
	if _, ok := overrides[def.Key]; ok {
		source = "db"
	}

	jsonOK(w, map[string]any{
		"key":              def.Key,
		"value":            value,
		"default":          def.Default,
		"source":           source,
		"runtime_editable": def.RuntimeEditable,
		"description":      def.Description,
		"valid_values":     def.ValidValues,
	})
}

// ConfigSet handles POST /json/config/set.
func (h *jsonApiHandler) ConfigSet(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	def := config.SettingDefByKey(req.Key)
	if def == nil {
		jsonErr(w, http.StatusNotFound, "unknown setting")
		return
	}
	if !def.RuntimeEditable {
		jsonErr(w, http.StatusBadRequest, "setting cannot be changed at runtime")
		return
	}

	if err := h.deps.Settings.Set(r.Context(), req.Key, req.Value); err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}

	actor := currentUserFromContext(r)
	db.Audit(r.Context(), h.deps.DB, actor.ID, "settings.updated", "setting", req.Key, req.Value)
	jsonOK(w, map[string]string{"status": "updated"})
}

// ConfigReset handles POST /json/config/reset.
func (h *jsonApiHandler) ConfigReset(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	def := config.SettingDefByKey(req.Key)
	if def == nil {
		jsonErr(w, http.StatusNotFound, "unknown setting")
		return
	}

	if err := h.deps.Settings.Delete(r.Context(), req.Key); err != nil {
		jsonErr(w, http.StatusInternalServerError, "failed to reset setting")
		return
	}

	actor := currentUserFromContext(r)
	db.Audit(r.Context(), h.deps.DB, actor.ID, "settings.reset", "setting", req.Key, "reverted to default")
	jsonOK(w, map[string]string{"status": "reset to default"})
}

// --- Audit endpoint ---

// AuditTail handles GET /json/audit/tail?n=25.
func (h *jsonApiHandler) AuditTail(w http.ResponseWriter, r *http.Request, params api.AuditTailParams) {
	n := 25
	if params.N != nil && *params.N > 0 && *params.N <= 500 {
		n = *params.N
	}

	entries, err := db.ListAuditEntries(r.Context(), h.deps.DB, n, 0)
	if err != nil {
		slog.Error("json api: failed to list audit entries", "error", err)
		jsonErr(w, http.StatusInternalServerError, "failed to list audit entries")
		return
	}

	type auditJSON struct {
		Actor      string `json:"actor"`
		Action     string `json:"action"`
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Details    string `json:"details"`
		CreatedAt  string `json:"created_at"`
	}

	result := make([]auditJSON, len(entries))
	for i, e := range entries {
		result[i] = auditJSON{
			Actor:      e.ActorUsername,
			Action:     e.Action,
			TargetType: e.TargetType,
			TargetID:   e.TargetID,
			Details:    e.Details,
			CreatedAt:  e.CreatedAt,
		}
	}

	jsonOK(w, map[string]any{"entries": result})
}

// --- Routes ---

// RegisterRoutes adds all /json/* routes to the given mux.
// Routing is generated from the OpenAPI spec via oapi-codegen; the guard
// middleware is applied globally to all generated routes.
func (h *jsonApiHandler) RegisterRoutes(mux *http.ServeMux) {
	api.HandlerWithOptions(h, api.StdHTTPServerOptions{
		BaseRouter:  mux,
		Middlewares: []api.MiddlewareFunc{h.jsonApiMiddleware},
	})
	// Docs endpoints (not in the OpenAPI spec, wired separately).
	mux.HandleFunc("GET /json/openapi.yaml", h.ServeOpenAPISpec)
	mux.HandleFunc("GET /json/", h.ServeDocs)
}

// jsonApiMiddleware is a global middleware applied to all generated routes.
// It checks that the json_api setting is enabled, authenticates the request,
// and enforces Content-Type on state-changing methods. Login is exempted from
// auth (it is the entry point) but still gated on the json_api setting.
func (h *jsonApiHandler) jsonApiMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if JSON API is enabled.
		if h.deps.Settings.Load().JSONApi != "admin" {
			http.NotFound(w, r)
			return
		}

		// Login is the entry point — skip auth/content-type checks.
		if r.URL.Path == "/json/login" {
			next.ServeHTTP(w, r)
			return
		}

		// Authenticate: try session cookie first, then Bearer token.
		var user *model.User

		if cookie, err := r.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" {
			sess, err := h.deps.Auth.GetSession(r.Context(), cookie.Value)
			if err == nil && sess != nil {
				u, err := h.deps.Auth.GetUser(r.Context(), sess.UserID)
				if err == nil && u != nil && !u.Disabled {
					user = u
				}
			}
		}

		if user == nil {
			if ah := r.Header.Get("Authorization"); strings.HasPrefix(ah, "Bearer ") {
				token := strings.TrimPrefix(ah, "Bearer ")
				sess, err := h.deps.Auth.GetSession(r.Context(), token)
				if err == nil && sess != nil {
					u, err := h.deps.Auth.GetUser(r.Context(), sess.UserID)
					if err == nil && u != nil && !u.Disabled {
						user = u
					}
				}
			}
		}

		if user == nil {
			jsonErr(w, http.StatusUnauthorized, "authentication required")
			return
		}
		if !user.IsAdmin {
			jsonErr(w, http.StatusForbidden, "admin access required")
			return
		}

		// Content-Type check for state-changing methods.
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
			ct := r.Header.Get("Content-Type")
			if !strings.HasPrefix(ct, "application/json") {
				jsonErr(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}

		ctx := context.WithValue(r.Context(), ctxKeyUser, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// currentUserFromContext retrieves the user stored in context by JSONApiGuard.
func currentUserFromContext(r *http.Request) *model.User {
	u, _ := r.Context().Value(ctxKeyUser).(*model.User)
	return u
}
