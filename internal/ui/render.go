package ui

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
)

// UserInfo holds minimal user data for display in templates.
type UserInfo struct {
	ID       int64
	Username string
	IsAdmin  bool
}

// PageData is the top-level data passed to every template render.
type PageData struct {
	Title            string
	CurrentUser      *UserInfo
	CSRFToken        string
	Flash            string
	Content          any
	RegistrationMode string
	ActiveNav        string
}

// Renderer parses and renders HTML templates from the embedded filesystem.
type Renderer struct {
	templates *template.Template
}

// NewRenderer parses all templates from the embedded FS and returns a Renderer.
func NewRenderer() *Renderer {
	funcMap := template.FuncMap{
		"eq": func(a, b string) bool { return a == b },
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(Templates,
		"templates/partials/header.html",
		"templates/partials/footer.html",
		"templates/layout.html",
		"templates/home.html",
		"templates/setup.html",
		"templates/login.html",
		"templates/register.html",
		"templates/admin.html",
		"templates/admin_sessions.html",
		"templates/admin_user.html",
		"templates/oauth_authorize.html",
		"templates/files.html",
		"templates/files_search.html",
		"templates/settings.html",
	)
	if err != nil {
		slog.Error("failed to parse templates", "error", err)
		panic("failed to parse templates: " + err.Error())
	}

	return &Renderer{templates: tmpl}
}

// Render executes the layout template with the given page data, where
// contentTemplate specifies which {{define "content"}} block to use.
func (r *Renderer) Render(w http.ResponseWriter, contentTemplate string, data PageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	// Clone and add a "content" template that delegates to the named content block.
	tmpl, err := r.templates.Clone()
	if err != nil {
		slog.Error("failed to clone template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create a wrapper template that invokes the named content block.
	wrapper := `{{define "content"}}{{template "` + contentTemplate + `" .}}{{end}}`
	if _, err := tmpl.Parse(wrapper); err != nil {
		slog.Error("failed to parse content wrapper", "error", err, "template", contentTemplate)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		slog.Error("failed to execute template", "error", err, "template", contentTemplate)
	}
}

// RenderTo renders to an arbitrary writer (useful for testing).
func (r *Renderer) RenderTo(w io.Writer, contentTemplate string, data PageData) error {
	tmpl, err := r.templates.Clone()
	if err != nil {
		return err
	}
	wrapper := `{{define "content"}}{{template "` + contentTemplate + `" .}}{{end}}`
	if _, err := tmpl.Parse(wrapper); err != nil {
		return err
	}
	return tmpl.ExecuteTemplate(w, "layout", data)
}
