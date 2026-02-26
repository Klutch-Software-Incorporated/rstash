package web

import (
	"net/http"

	"gosilo/api"
)

// jsonRoute describes a JSON API endpoint for mux wiring.
type jsonRoute struct {
	Method  string           // "GET" or "POST"
	Path    string           // e.g. "/json/user/list"
	Handler http.HandlerFunc // the actual handler function
	Guard   bool             // true = wrap with JSONApiGuard
}

// routes returns the full registry of JSON API endpoints.
func (h *jsonApiHandler) routes() []jsonRoute {
	return []jsonRoute{
		// Authentication
		{Method: "POST", Path: "/json/login", Handler: h.Login, Guard: false},
		{Method: "POST", Path: "/json/logout", Handler: h.Logout, Guard: true},
		// User Management
		{Method: "GET", Path: "/json/user/list", Handler: h.UserList, Guard: true},
		{Method: "POST", Path: "/json/user/add", Handler: h.UserAdd, Guard: true},
		{Method: "POST", Path: "/json/user/delete", Handler: h.UserDelete, Guard: true},
		{Method: "POST", Path: "/json/user/passwd", Handler: h.UserPasswd, Guard: true},
		{Method: "POST", Path: "/json/user/promote", Handler: h.UserPromote, Guard: true},
		{Method: "POST", Path: "/json/user/disable", Handler: h.UserDisable, Guard: true},
		// Configuration
		{Method: "GET", Path: "/json/config/list", Handler: h.ConfigList, Guard: true},
		{Method: "GET", Path: "/json/config/get", Handler: h.ConfigGet, Guard: true},
		{Method: "POST", Path: "/json/config/set", Handler: h.ConfigSet, Guard: true},
		{Method: "POST", Path: "/json/config/reset", Handler: h.ConfigReset, Guard: true},
		// Audit
		{Method: "GET", Path: "/json/audit/tail", Handler: h.AuditTail, Guard: true},
	}
}

// ServeOpenAPISpec serves the raw OpenAPI YAML spec at GET /json/openapi.yaml.
func (h *jsonApiHandler) ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	if h.deps.Settings.Load().JSONApi != "admin" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.Write(api.OpenAPISpec)
}

// ServeDocs serves a minimal HTML page that loads the Scalar API reference UI.
func (h *jsonApiHandler) ServeDocs(w http.ResponseWriter, r *http.Request) {
	if h.deps.Settings.Load().JSONApi != "admin" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(docsHTML))
}

const docsHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Gosilo JSON API Reference</title>
</head>
<body>
  <script id="api-reference" data-url="/json/openapi.yaml"></script>
  <script src="/static/scalar.min.js"></script>
</body>
</html>`
