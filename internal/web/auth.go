package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"gosilo/internal/auth"
	"gosilo/internal/ui"
)

type authHandler struct {
	deps *UIDeps
}

// AuthHandler returns handler methods for login/logout.
func AuthHandler(deps *UIDeps) *authHandler {
	return &authHandler{deps: deps}
}

type loginContent struct {
	Username   string
	Error      string
	RedirectTo string
}

func (h *authHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	redirectTo := r.URL.Query().Get("redirect")

	h.deps.Renderer.Render(w, "login", h.deps.pageData(w, r, "Login — Gosilo", &loginContent{RedirectTo: redirectTo}))
}

func (h *authHandler) DoLogin(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "login", ui.PageData{
			Title:            "Login — Gosilo",
			Content:          &loginContent{Username: username, Error: msg},
			RegistrationMode: h.deps.Config.RegistrationMode,
		})
	}

	user, err := h.deps.Auth.Authenticate(r.Context(), username, password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			renderErr("Invalid username or password.")
		} else {
			slog.Error("failed to authenticate", "error", err)
			renderErr("An error occurred. Please try again.")
		}
		return
	}

	sess, err := h.deps.Auth.CreateSession(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		renderErr("An error occurred. Please try again.")
		return
	}

	auth.SetSessionCookie(w, sess.Token)

	// Redirect to the originally requested page, or home.
	redirectTo := r.FormValue("redirect")
	if redirectTo == "" || !strings.HasPrefix(redirectTo, "/") {
		redirectTo = "/"
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (h *authHandler) DoLogout(w http.ResponseWriter, r *http.Request) {
	sess := CurrentSession(r)
	if sess != nil {
		if err := h.deps.Auth.DestroySession(r.Context(), sess.Token); err != nil {
			slog.Error("failed to delete session", "error", err)
		}
	}

	auth.ClearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
