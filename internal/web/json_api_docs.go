package web

import (
	"net/http"

	"gosilo/api"
)

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
  <redoc spec-url="/json/openapi.yaml"></redoc>
  <script src="/static/redoc.standalone.js"></script>
</body>
</html>`
