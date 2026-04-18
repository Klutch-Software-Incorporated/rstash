package web

import "net/http"

type apiDocsHandler struct {
	deps *UIDeps
}

// APIDocsHandler returns the admin OpenAPI docs handler.
func APIDocsHandler(deps *UIDeps) *apiDocsHandler {
	return &apiDocsHandler{deps: deps}
}

// ShowAPIDocs renders GET /admin/api-docs — a full-page Swagger UI view of the
// live /api/admin/openapi.json spec. Rendered standalone (no admin chrome) so
// it can be opened in its own tab from the admin nav.
func (h *apiDocsHandler) ShowAPIDocs(w http.ResponseWriter, r *http.Request) {
	h.deps.Renderer.RenderStandalone(w, "admin_api_docs", map[string]any{
		"Title": "rstash admin API — " + r.Host,
	})
}
