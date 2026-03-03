package web

import (
	"log/slog"
	"net/http"

	"gosilo/internal/licenses"
)

type legalHandler struct {
	deps *UIDeps
}

// LegalHandler returns handler methods for legal pages (TOS, Privacy).
func LegalHandler(deps *UIDeps) *legalHandler {
	return &legalHandler{deps: deps}
}

type legalContent struct {
	Title string
	Body  string
}

// ShowTerms handles GET /legal/terms.
func (h *legalHandler) ShowTerms(w http.ResponseWriter, r *http.Request) {
	snap := h.deps.Settings.Load()

	switch snap.TOSMode {
	case "off":
		http.NotFound(w, r)
		return
	case "url":
		http.Redirect(w, r, snap.TOSContent, http.StatusFound)
		return
	default: // "text"
		content := &legalContent{
			Title: "Terms of Service",
			Body:  snap.TOSContent,
		}
		h.deps.Renderer.Render(w, "legal", h.deps.pageData(w, r, "Terms of Service — Gosilo", content))
	}
}

type licensesContent struct {
	Project  licenses.ProjectLicense
	Direct   []licenses.DependencyLicense
	Indirect []licenses.DependencyLicense
	Total    int
}

// ShowLicenses handles GET /legal/licenses.
func (h *legalHandler) ShowLicenses(w http.ResponseWriter, r *http.Request) {
	data, err := licenses.Load()
	if err != nil {
		slog.Error("failed to load license data", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var direct, indirect []licenses.DependencyLicense
	for _, dep := range data.Dependencies {
		if dep.Direct {
			direct = append(direct, dep)
		} else {
			indirect = append(indirect, dep)
		}
	}

	content := &licensesContent{
		Project:  data.Project,
		Direct:   direct,
		Indirect: indirect,
		Total:    len(data.Dependencies),
	}
	h.deps.Renderer.Render(w, "licenses",
		h.deps.pageData(w, r, "Open Source Licenses — Gosilo", content))
}

// ShowPrivacy handles GET /legal/privacy.
func (h *legalHandler) ShowPrivacy(w http.ResponseWriter, r *http.Request) {
	snap := h.deps.Settings.Load()

	switch snap.PrivacyMode {
	case "off":
		http.NotFound(w, r)
		return
	case "url":
		http.Redirect(w, r, snap.PrivacyContent, http.StatusFound)
		return
	default: // "text"
		content := &legalContent{
			Title: "Privacy Policy",
			Body:  snap.PrivacyContent,
		}
		h.deps.Renderer.Render(w, "legal", h.deps.pageData(w, r, "Privacy Policy — Gosilo", content))
	}
}
