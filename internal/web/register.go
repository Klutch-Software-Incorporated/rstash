package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gosilo/internal/auth"
	"gosilo/internal/db"
	"gosilo/internal/ui"
)

type registerHandler struct {
	deps *UIDeps
}

// RegisterHandler returns handler methods for user registration.
func RegisterHandler(deps *UIDeps) *registerHandler {
	return &registerHandler{deps: deps}
}

type registerContent struct {
	Username string
	Closed   bool
	Error    string
}

func (h *registerHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	mode := h.deps.Settings.Load().RegistrationMode
	content := &registerContent{
		Closed: mode == "closed",
	}

	h.deps.Renderer.Render(w, "register", h.deps.pageData(w, r, "Register — Gosilo", content))
}

func (h *registerHandler) DoRegister(w http.ResponseWriter, r *http.Request) {
	mode := h.deps.Settings.Load().RegistrationMode

	if mode == "closed" {
		http.Error(w, "Registration is closed", http.StatusForbidden)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "register", ui.PageData{
			Title: "Register — Gosilo",
			Content: &registerContent{
				Username: username,
				Error:    msg,
			},
			RegistrationMode: mode,
		})
	}

	if username == "" {
		renderErr("Username is required.")
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

	// Create user.
	user, err := h.deps.Auth.CreateUser(r.Context(), username, password, false)
	if err != nil {
		slog.Error("failed to create user", "error", err)
		renderErr("Failed to create user. Username may already be taken.")
		return
	}

	// Create session.
	sess, err := h.deps.Auth.CreateSession(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	db.Audit(r.Context(), h.deps.DB, user.ID, "user.registered", "user", fmt.Sprintf("%d", user.ID), username)
	auth.SetSessionCookie(w, sess.Token, h.deps.SecureCookies)
	ui.SetFlash(w, "Account created successfully.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
