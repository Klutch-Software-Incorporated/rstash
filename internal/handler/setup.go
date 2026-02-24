package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"gosilo/internal/db"
	"gosilo/internal/ui"
)

type setupHandler struct {
	deps *UIDeps
}

// SetupHandler returns handler methods for the setup wizard.
func SetupHandler(deps *UIDeps) *setupHandler {
	return &setupHandler{deps: deps}
}

type setupContent struct {
	Username string
	Error    string
}

func (h *setupHandler) ShowSetup(w http.ResponseWriter, r *http.Request) {
	// If users already exist, redirect to home.
	count, err := db.UserCount(r.Context(), h.deps.DB)
	if err != nil {
		slog.Error("failed to check user count", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	h.deps.Renderer.Render(w, "setup", ui.PageData{
		Title:   "Setup — Gosilo",
		Content: &setupContent{},
	})
}

func (h *setupHandler) DoSetup(w http.ResponseWriter, r *http.Request) {
	// If users already exist, redirect to home.
	count, err := db.UserCount(r.Context(), h.deps.DB)
	if err != nil {
		slog.Error("failed to check user count", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "setup", ui.PageData{
			Title:   "Setup — Gosilo",
			Content: &setupContent{Username: username, Error: msg},
		})
	}

	if username == "" {
		renderErr("Username is required.")
		return
	}
	if len(password) < 8 {
		renderErr("Password must be at least 8 characters.")
		return
	}
	if password != confirm {
		renderErr("Passwords do not match.")
		return
	}

	user, err := db.CreateUser(r.Context(), h.deps.DB, username, password, true)
	if err != nil {
		slog.Error("failed to create admin user", "error", err)
		renderErr("Failed to create user. Username may already be taken.")
		return
	}

	// Create session.
	sess, err := db.CreateSession(r.Context(), h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	SetSessionCookie(w, sess.Token)
	SetFlash(w, "Welcome to Gosilo! Your admin account has been created.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
