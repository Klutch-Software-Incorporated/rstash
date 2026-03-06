package web

import (
	"fmt"
	"log/slog"
	"net/http"

	"rstash/internal/auth"
	"rstash/internal/db"
	"rstash/internal/settings"
	"rstash/internal/ui"
)

type registerHandler struct {
	deps *UIDeps
}

// RegisterHandler returns handler methods for user registration.
func RegisterHandler(deps *UIDeps) *registerHandler {
	return &registerHandler{deps: deps}
}

type registerContent struct {
	Username     string
	Closed       bool
	ApprovalMode bool
	Success      bool
	Error        string
	TOSUrl       string // non-empty if TOS is active
	PrivacyUrl   string // non-empty if Privacy Policy is active
}

func (h *registerHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	snap := h.deps.Settings.Load()
	content := &registerContent{
		Closed:       snap.RegistrationMode == "closed",
		ApprovalMode: snap.RegistrationMode == "approval",
	}
	content.TOSUrl, content.PrivacyUrl = h.legalURLs(snap)

	h.deps.Renderer.Render(w, "register", h.deps.pageData(w, r, "Register — rstash", content))
}

func (h *registerHandler) DoRegister(w http.ResponseWriter, r *http.Request) {
	snap := h.deps.Settings.Load()

	if snap.RegistrationMode != "open" && snap.RegistrationMode != "approval" {
		http.Error(w, "Registration is closed", http.StatusForbidden)
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	username, valErr := db.ValidateUsername(r.FormValue("username"))

	tosUrl, privacyUrl := h.legalURLs(snap)

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "register", ui.PageData{
			Title: "Register — rstash",
			Content: &registerContent{
				Username:   username,
				Error:      msg,
				TOSUrl:     tosUrl,
				PrivacyUrl: privacyUrl,
			},
			RegistrationMode: snap.RegistrationMode,
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

	// Create user — in approval mode, the account is not yet approved.
	approved := snap.RegistrationMode == "open"
	user, err := h.deps.Auth.CreateUser(r.Context(), username, password, false, approved)
	if err != nil {
		slog.Error("failed to create user", "error", err)
		renderErr("Failed to create user. Username may already be taken.")
		return
	}

	// Record TOS and Privacy Policy acceptance.
	if tosActive {
		if err := h.deps.Repo.AcceptTOS(r.Context(), user.ID); err != nil {
			slog.Error("failed to record TOS acceptance", "error", err)
		}
	}
	if privacyActive {
		if err := h.deps.Repo.AcceptPrivacy(r.Context(), user.ID); err != nil {
			slog.Error("failed to record Privacy acceptance", "error", err)
		}
	}

	if !approved {
		// Approval mode: show success message, don't create a session.
		h.deps.Repo.Audit(r.Context(), user.ID, "user.registered_pending", "user", fmt.Sprintf("%d", user.ID), username)
		h.deps.Renderer.Render(w, "register", h.deps.pageData(w, r, "Register — rstash", &registerContent{
			ApprovalMode: true,
			Success:      true,
			TOSUrl:       tosUrl,
			PrivacyUrl:   privacyUrl,
		}))
		return
	}

	// Create session.
	sess, err := h.deps.Auth.CreateSession(r.Context(), user.ID, ClientIP(r))
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.deps.Repo.Audit(r.Context(), user.ID, "user.registered", "user", fmt.Sprintf("%d", user.ID), username)
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
