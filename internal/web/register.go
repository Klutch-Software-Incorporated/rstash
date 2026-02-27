package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gosilo/internal/auth"
	"gosilo/internal/db"
	"gosilo/internal/settings"
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
	Username   string
	Closed     bool
	Error      string
	TOSUrl     string // non-empty if TOS is active
	PrivacyUrl string // non-empty if Privacy Policy is active
}

func (h *registerHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	snap := h.deps.Settings.Load()
	content := &registerContent{
		Closed: snap.RegistrationMode == "closed",
	}
	content.TOSUrl, content.PrivacyUrl = h.legalURLs(snap)

	h.deps.Renderer.Render(w, "register", h.deps.pageData(w, r, "Register — Gosilo", content))
}

func (h *registerHandler) DoRegister(w http.ResponseWriter, r *http.Request) {
	snap := h.deps.Settings.Load()

	if snap.RegistrationMode == "closed" {
		http.Error(w, "Registration is closed", http.StatusForbidden)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	tosUrl, privacyUrl := h.legalURLs(snap)

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "register", ui.PageData{
			Title: "Register — Gosilo",
			Content: &registerContent{
				Username:   username,
				Error:      msg,
				TOSUrl:     tosUrl,
				PrivacyUrl: privacyUrl,
			},
			RegistrationMode: snap.RegistrationMode,
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

	// Require TOS acceptance if TOS is active.
	tosActive := snap.TOSMode != "off"
	privacyActive := snap.PrivacyMode != "off"
	if tosActive && r.FormValue("tos_accept") != "on" {
		renderErr("You must accept the Terms of Service to register.")
		return
	}
	// Require Privacy acceptance when Privacy is active but TOS is not
	// (when both are active, the TOS checkbox implicitly covers both).
	if privacyActive && !tosActive && r.FormValue("privacy_accept") != "on" {
		renderErr("You must accept the Privacy Policy to register.")
		return
	}

	// Create user.
	user, err := h.deps.Auth.CreateUser(r.Context(), username, password, false)
	if err != nil {
		slog.Error("failed to create user", "error", err)
		renderErr("Failed to create user. Username may already be taken.")
		return
	}

	// Record TOS and Privacy Policy acceptance.
	if tosActive {
		if err := db.AcceptTOS(r.Context(), h.deps.DB, user.ID); err != nil {
			slog.Error("failed to record TOS acceptance", "error", err)
		}
	}
	if privacyActive {
		if err := db.AcceptPrivacy(r.Context(), h.deps.DB, user.ID); err != nil {
			slog.Error("failed to record Privacy acceptance", "error", err)
		}
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

// legalURLs returns resolved TOS and Privacy URLs based on the current settings.
func (h *registerHandler) legalURLs(snap *settings.Snapshot) (string, string) {
	var tosUrl, privacyUrl string
	if snap.TOSMode == "url" {
		tosUrl = snap.TOSContent
	} else if snap.TOSMode == "text" {
		tosUrl = "/legal/terms"
	}
	if snap.PrivacyMode == "url" {
		privacyUrl = snap.PrivacyContent
	} else if snap.PrivacyMode == "text" {
		privacyUrl = "/legal/privacy"
	}
	return tosUrl, privacyUrl
}
