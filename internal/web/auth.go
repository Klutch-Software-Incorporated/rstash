package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"rstash/internal/auth"
	"rstash/internal/db"
	"rstash/internal/metrics"
	"rstash/internal/ui"
)

type authHandler struct {
	deps *UIDeps
}

// AuthHandler returns handler methods for login/logout.
func AuthHandler(deps *UIDeps) *authHandler {
	return &authHandler{deps: deps}
}

type loginContent struct {
	Username         string
	Error            string
	DisabledMessage  string // operator-configured HTML shown when account is disabled
	RedirectTo       string
}

func (h *authHandler) ShowLogin(w http.ResponseWriter, r *http.Request) {
	if CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	redirectTo := r.URL.Query().Get("redirect")

	h.deps.Renderer.Render(w, "login", h.deps.pageData(w, r, "Login — rstash", &loginContent{RedirectTo: redirectTo}))
}

func (h *authHandler) DoLogin(w http.ResponseWriter, r *http.Request) {
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")

	renderErr := func(msg string, disabledHTML string) {
		h.deps.Renderer.Render(w, "login", ui.PageData{
			Title:            "Login — rstash",
			Content:          &loginContent{Username: username, Error: msg, DisabledMessage: disabledHTML},
			RegistrationMode: h.deps.Config.RegistrationMode,
		})
	}

	user, err := h.deps.Auth.Authenticate(r.Context(), username, password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			metrics.LoginFailuresTotal.Inc()
			h.deps.Repo.Audit(r.Context(), db.SystemActorID, "auth.login_failed", "user", username, "invalid credentials")
			renderErr("Invalid username or password.", "")
		} else if errors.Is(err, auth.ErrAccountPendingApproval) {
			renderErr("Your account is pending approval.", "")
		} else if errors.Is(err, auth.ErrAccountDisabled) {
			renderErr("Your account has been disabled.", h.deps.Settings.Load().DisabledAccountMessage)
		} else {
			slog.Error("failed to authenticate", "error", err)
			renderErr("An error occurred. Please try again.", "")
		}
		return
	}

	sess, err := h.deps.Auth.CreateSession(r.Context(), user.ID, h.deps.ClientIPForStorage(r))
	if err != nil {
		slog.Error("failed to create session", "error", err)
		renderErr("An error occurred. Please try again.", "")
		return
	}

	if err := h.deps.Repo.UpdateUserLastLogin(r.Context(), user.ID, h.deps.ClientIPForStorage(r)); err != nil {
		slog.Error("failed to update last login", "error", err)
	}

	h.deps.Repo.Audit(r.Context(), user.ID, "auth.login", "user", fmt.Sprintf("%d", user.ID), username)
	auth.SetSessionCookie(w, sess.Token, h.deps.Settings.Load().CookieDomain, h.deps.SecureCookies)

	// Redirect to the originally requested page, or home.
	redirectTo := r.FormValue("redirect")
	if redirectTo == "" || !strings.HasPrefix(redirectTo, "/") {
		redirectTo = "/"
	}
	http.Redirect(w, r, redirectTo, http.StatusSeeOther)
}

func (h *authHandler) DoLogout(w http.ResponseWriter, r *http.Request) {
	user := CurrentUser(r)
	sess := CurrentSession(r)
	if sess != nil {
		if err := h.deps.Auth.DestroySession(r.Context(), sess.Token); err != nil {
			slog.Error("failed to delete session", "error", err)
		}
	}

	if user != nil {
		h.deps.Repo.Audit(r.Context(), user.ID, "auth.logout", "user", fmt.Sprintf("%d", user.ID), user.Username)
	}

	auth.ClearSessionCookie(w, h.deps.Settings.Load().CookieDomain, h.deps.SecureCookies)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
