package web

import "net/http"

type apiDocsHandler struct {
	deps *UIDeps
}

// APIDocsHandler returns the admin OpenAPI docs handler.
func APIDocsHandler(deps *UIDeps) *apiDocsHandler {
	return &apiDocsHandler{deps: deps}
}

// ShowAPIDocs renders GET /admin/api-docs — a Redoc-powered view of the live
// /api/admin/openapi.json spec.
func (h *apiDocsHandler) ShowAPIDocs(w http.ResponseWriter, r *http.Request) {
	h.deps.Renderer.Render(w, "admin_api_docs", h.deps.adminPageData(w, r, "API Docs — Admin", "api-docs", nil))
}
