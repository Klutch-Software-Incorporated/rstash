package handler

import (
	"html/template"
	"log/slog"
	"net/http"

	"gosilo/internal/ui"
)

// UI returns an http.Handler that serves the web UI routes.
func UI() http.Handler {
	tmpl, err := template.ParseFS(ui.Templates, "templates/layout.html")
	if err != nil {
		slog.Error("failed to parse templates", "error", err)
		panic("failed to parse templates: " + err.Error())
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		tmpl.Execute(w, map[string]string{"Title": "Gosilo", "Content": "Welcome to Gosilo — a remoteStorage server."})
	})

	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "admin: not yet implemented", http.StatusNotImplemented)
	})

	mux.HandleFunc("GET /setup", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "setup: not yet implemented", http.StatusNotImplemented)
	})

	return mux
}
