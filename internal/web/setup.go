package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"gosilo/internal/auth"
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
	count, err := h.deps.Auth.UserCount(r.Context())
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
	count, err := h.deps.Auth.UserCount(r.Context())
	if err != nil {
		slog.Error("failed to check user count", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	username, valErr := db.ValidateUsername(r.FormValue("username"))

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "setup", ui.PageData{
			Title:   "Setup — Gosilo",
			Content: &setupContent{Username: username, Error: msg},
		})
	}

	if valErr != nil {
		renderErr(valErr.Error())
		return
	}
	if msg := validatePassword(password); msg != "" {
		renderErr(msg)
		return
	}
	if password != confirm {
		renderErr("Passwords do not match.")
		return
	}

	user, err := h.deps.Auth.CreateUser(r.Context(), username, password, true, true)
	if err != nil {
		slog.Error("failed to create admin user", "error", err)
		renderErr("Failed to create user. Username may already be taken.")
		return
	}

	// Auto-accept TOS/Privacy for the initial admin (operator-created).
	_ = h.deps.Repo.AcceptTOS(r.Context(), user.ID)
	_ = h.deps.Repo.AcceptPrivacy(r.Context(), user.ID)

	// Create session.
	sess, err := h.deps.Auth.CreateSession(r.Context(), user.ID, ClientIP(r))
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.deps.Repo.Audit(r.Context(), user.ID, "setup.completed", "user", fmt.Sprintf("%d", user.ID), username)
	auth.SetSessionCookie(w, sess.Token, h.deps.SecureCookies)
	ui.SetFlash(w, "Welcome to Gosilo! Your admin account has been created.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
