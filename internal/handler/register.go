package handler

import (
	"log/slog"
	"net/http"
	"strings"

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
	Username   string
	InviteCode string
	InviteMode bool
	Closed     bool
	Error      string
}

func (h *registerHandler) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if CurrentUser(r) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	mode := h.deps.Config.RegistrationMode
	content := &registerContent{
		Closed:     mode == "closed",
		InviteMode: mode == "invite",
	}

	h.deps.Renderer.Render(w, "register", ui.PageData{
		Title:            "Register — Gosilo",
		Flash:            GetFlash(w, r),
		Content:          content,
		RegistrationMode: mode,
	})
}

func (h *registerHandler) DoRegister(w http.ResponseWriter, r *http.Request) {
	mode := h.deps.Config.RegistrationMode

	if mode == "closed" {
		http.Error(w, "Registration is closed", http.StatusForbidden)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")
	inviteCode := strings.TrimSpace(r.FormValue("invite_code"))

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "register", ui.PageData{
			Title: "Register — Gosilo",
			Content: &registerContent{
				Username:   username,
				InviteCode: inviteCode,
				InviteMode: mode == "invite",
				Error:      msg,
			},
			RegistrationMode: mode,
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

	// Check invite code if needed.
	if mode == "invite" {
		if inviteCode == "" {
			renderErr("Invite code is required.")
			return
		}
		inv, err := db.GetInviteCode(r.Context(), h.deps.DB, inviteCode)
		if err != nil {
			slog.Error("failed to look up invite code", "error", err)
			renderErr("An error occurred. Please try again.")
			return
		}
		if inv == nil || inv.UsedBy != nil {
			renderErr("Invalid or already used invite code.")
			return
		}
	}

	// Create user.
	user, err := db.CreateUser(r.Context(), h.deps.DB, username, password, false)
	if err != nil {
		slog.Error("failed to create user", "error", err)
		renderErr("Failed to create user. Username may already be taken.")
		return
	}

	// Redeem invite code if applicable.
	if mode == "invite" {
		if err := db.RedeemInviteCode(r.Context(), h.deps.DB, inviteCode, user.ID); err != nil {
			slog.Error("failed to redeem invite code", "error", err)
			// User was created but invite redeem failed — still log them in.
		}
	}

	// Create session.
	sess, err := db.CreateSession(r.Context(), h.deps.DB, user.ID)
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	SetSessionCookie(w, sess.Token)
	SetFlash(w, "Account created successfully.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
