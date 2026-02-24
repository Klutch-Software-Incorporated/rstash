package handler

import (
	"log/slog"
	"net/http"
	"strings"

	"gosilo/internal/db"
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

	h.deps.Renderer.Render(w, "login", ui.PageData{
		Title:            "Login — Gosilo",
		Flash:            GetFlash(w, r),
		Content:          &loginContent{RedirectTo: redirectTo},
		RegistrationMode: h.deps.Config.RegistrationMode,
	})
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

	user, err := db.GetUserByUsername(r.Context(), h.deps.DB, username)
	if err != nil {
		slog.Error("failed to look up user", "error", err)
		renderErr("An error occurred. Please try again.")
		return
	}
	if user == nil || !db.CheckPassword(user, password) {
		renderErr("Invalid username or password.")
		return
	}

	sess, err := db.CreateSession(r.Context(), h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		renderErr("An error occurred. Please try again.")
		return
	}

	SetSessionCookie(w, sess.Token)

	// Redirect to the originally requested page, or home.
	redirectTo := r.FormValue("redirect")
	if redirectTo == "" || !strings.HasPrefix(redirectTo, "/") {
		redirectTo = "/"
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (h *authHandler) DoLogout(w http.ResponseWriter, r *http.Request) {
	if !ValidateCSRF(r) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	sess := CurrentSession(r)
	if sess != nil {
		if err := db.DeleteSession(r.Context(), h.deps.DB, sess.Token); err != nil {
			slog.Error("failed to delete session", "error", err)
		}
	}

	ClearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
