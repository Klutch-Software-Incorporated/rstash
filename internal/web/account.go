package web

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"rstash/internal/db"
	"rstash/internal/email"
	"rstash/internal/metrics"
	"rstash/internal/ui"
)

type accountHandler struct {
	deps *UIDeps
}

// AccountHandler returns handler methods for account management (add email, verification, password reset).
func AccountHandler(deps *UIDeps) *accountHandler {
	return &accountHandler{deps: deps}
}

// --- Add Email ---

type addEmailContent struct {
	Error string
}

func (h *accountHandler) ShowAddEmail(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)
	if user.Email != nil && *user.Email != "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	h.deps.Renderer.Render(w, "add_email", h.deps.pageData(w, r, "Add Email — rstash", &addEmailContent{}))
}

func (h *accountHandler) DoAddEmail(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)
	if user.Email != nil && *user.Email != "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	emailRaw := r.FormValue("email")
	emailAddr, err := db.ValidateEmail(emailRaw)
	if err != nil {
		h.deps.Renderer.Render(w, "add_email", h.deps.pageData(w, r, "Add Email — rstash", &addEmailContent{Error: err.Error()}))
		return
	}

	// Check uniqueness.
	existing, err := h.deps.Auth.GetUserByEmail(r.Context(), emailAddr)
	if err != nil {
		slog.Error("failed to check email uniqueness", "error", err)
		h.deps.Renderer.Render(w, "add_email", h.deps.pageData(w, r, "Add Email — rstash", &addEmailContent{Error: "An error occurred. Please try again."}))
		return
	}
	if existing != nil {
		h.deps.Renderer.Render(w, "add_email", h.deps.pageData(w, r, "Add Email — rstash", &addEmailContent{Error: "An account with this email already exists."}))
		return
	}

	if err := h.deps.Auth.UpdateEmail(r.Context(), user.ID, emailAddr); err != nil {
		slog.Error("failed to update user email", "error", err)
		h.deps.Renderer.Render(w, "add_email", h.deps.pageData(w, r, "Add Email — rstash", &addEmailContent{Error: "Failed to save email."}))
		return
	}

	h.deps.Repo.Audit(r.Context(), user.ID, "user.email_added", "user", fmt.Sprintf("%d", user.ID), emailAddr)

	// Trigger verification if mailer is configured.
	if h.deps.Mailer != nil {
		h.sendVerificationEmail(r, user.ID, emailAddr)
	}

	ui.SetFlash(w, "Email address saved.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// --- Email Verification ---

func (h *accountHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		h.deps.Renderer.Render(w, "verify_email", h.deps.pageData(w, r, "Verify Email — rstash", &verifyEmailContent{Error: "Missing verification token."}))
		return
	}

	user, err := h.deps.Repo.GetUserByEmailVerifyToken(r.Context(), token)
	if err != nil {
		slog.Error("failed to look up verification token", "error", err)
		h.deps.Renderer.Render(w, "verify_email", h.deps.pageData(w, r, "Verify Email — rstash", &verifyEmailContent{Error: "An error occurred."}))
		return
	}
	if user == nil {
		h.deps.Renderer.Render(w, "verify_email", h.deps.pageData(w, r, "Verify Email — rstash", &verifyEmailContent{Error: "Invalid or expired verification link."}))
		return
	}

	if err := h.deps.Repo.VerifyUserEmail(r.Context(), user.ID); err != nil {
		slog.Error("failed to verify email", "error", err)
		h.deps.Renderer.Render(w, "verify_email", h.deps.pageData(w, r, "Verify Email — rstash", &verifyEmailContent{Error: "An error occurred."}))
		return
	}

	h.deps.Repo.Audit(r.Context(), user.ID, "user.email_verified", "user", fmt.Sprintf("%d", user.ID), "")
	h.deps.Renderer.Render(w, "verify_email", h.deps.pageData(w, r, "Verify Email — rstash", &verifyEmailContent{Success: true}))
}

type verifyEmailContent struct {
	Success bool
	Error   string
}

func (h *accountHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	if !RequireAuth(w, r) {
		return
	}
	user := CurrentUser(r)
	if user.Email == nil || *user.Email == "" || user.EmailVerified {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if h.deps.Mailer == nil {
		http.Error(w, "Email not configured", http.StatusNotFound)
		return
	}

	// Rate limit: if there's a token with expiry within the last 5 minutes of being issued
	// (i.e. expiry is more than 47h55m away), deny.
	if user.EmailVerifyExpiry != nil {
		remaining := time.Until(*user.EmailVerifyExpiry)
		if remaining > 47*time.Hour+55*time.Minute {
			ui.SetFlashError(w, "Please wait a few minutes before requesting another verification email.")
			http.Redirect(w, r, "/~"+user.Username+"/settings", http.StatusSeeOther)
			return
		}
	}

	h.sendVerificationEmail(r, user.ID, *user.Email)
	ui.SetFlash(w, "Verification email sent.")
	http.Redirect(w, r, "/~"+user.Username+"/settings", http.StatusSeeOther)
}

// --- Password Reset ---

type forgotPasswordContent struct {
	Success bool
	Error   string
}

type resetPasswordContent struct {
	Token   string
	Error   string
	Success bool
}

func (h *accountHandler) ShowForgotPassword(w http.ResponseWriter, r *http.Request) {
	if h.deps.Mailer == nil {
		http.NotFound(w, r)
		return
	}
	h.deps.Renderer.Render(w, "forgot_password", h.deps.pageData(w, r, "Forgot Password — rstash", &forgotPasswordContent{}))
}

func (h *accountHandler) DoForgotPassword(w http.ResponseWriter, r *http.Request) {
	if h.deps.Mailer == nil {
		http.NotFound(w, r)
		return
	}

	emailAddr := r.FormValue("email")

	// Always show success to avoid leaking email existence.
	showSuccess := func() {
		h.deps.Renderer.Render(w, "forgot_password", h.deps.pageData(w, r, "Forgot Password — rstash", &forgotPasswordContent{Success: true}))
	}

	if emailAddr == "" {
		showSuccess()
		return
	}

	user, err := h.deps.Auth.GetUserByEmail(r.Context(), emailAddr)
	if err != nil {
		slog.Error("failed to look up user by email for password reset", "error", err)
		showSuccess()
		return
	}
	if user == nil {
		showSuccess()
		return
	}

	token := generateToken()
	expiry := time.Now().UTC().Add(1 * time.Hour)
	if err := h.deps.Repo.SetPasswordResetToken(r.Context(), user.ID, token, expiry); err != nil {
		slog.Error("failed to set password reset token", "error", err)
		showSuccess()
		return
	}

	msg := email.PasswordResetEmail(*user.Email, h.deps.Config.BaseURL, token)
	if err := h.deps.Mailer.Send(r.Context(), msg); err != nil {
		slog.Error("failed to send password reset email", "error", err)
		metrics.EmailsSentTotal.WithLabelValues("reset", "failed").Inc()
		_ = h.deps.Repo.LogEmailSend(r.Context(), *user.Email, "reset", msg.Subject, "failed", err.Error())
	} else {
		metrics.EmailsSentTotal.WithLabelValues("reset", "sent").Inc()
		_ = h.deps.Repo.LogEmailSend(r.Context(), *user.Email, "reset", msg.Subject, "sent", "")
	}

	h.deps.Repo.Audit(r.Context(), user.ID, "user.password_reset_requested", "user", fmt.Sprintf("%d", user.ID), "")
	showSuccess()
}

func (h *accountHandler) ShowResetPassword(w http.ResponseWriter, r *http.Request) {
	if h.deps.Mailer == nil {
		http.NotFound(w, r)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		h.deps.Renderer.Render(w, "reset_password", h.deps.pageData(w, r, "Reset Password — rstash", &resetPasswordContent{Error: "Missing reset token."}))
		return
	}

	user, err := h.deps.Repo.GetUserByPasswordResetToken(r.Context(), token)
	if err != nil || user == nil {
		h.deps.Renderer.Render(w, "reset_password", h.deps.pageData(w, r, "Reset Password — rstash", &resetPasswordContent{Error: "Invalid or expired reset link."}))
		return
	}

	h.deps.Renderer.Render(w, "reset_password", h.deps.pageData(w, r, "Reset Password — rstash", &resetPasswordContent{Token: token}))
}

func (h *accountHandler) DoResetPassword(w http.ResponseWriter, r *http.Request) {
	if h.deps.Mailer == nil {
		http.NotFound(w, r)
		return
	}

	token := r.FormValue("token")
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "reset_password", h.deps.pageData(w, r, "Reset Password — rstash", &resetPasswordContent{Token: token, Error: msg}))
	}

	if token == "" {
		renderErr("Missing reset token.")
		return
	}

	user, err := h.deps.Repo.GetUserByPasswordResetToken(r.Context(), token)
	if err != nil || user == nil {
		renderErr("Invalid or expired reset link.")
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

	if err := h.deps.Auth.UpdatePassword(r.Context(), user.ID, password); err != nil {
		slog.Error("failed to update password via reset", "error", err)
		renderErr("An error occurred. Please try again.")
		return
	}

	_ = h.deps.Repo.ClearPasswordResetToken(r.Context(), user.ID)
	_ = h.deps.Auth.TerminateAllSessions(r.Context(), user.ID)

	h.deps.Repo.Audit(r.Context(), user.ID, "user.password_reset", "user", fmt.Sprintf("%d", user.ID), "")

	h.deps.Renderer.Render(w, "reset_password", h.deps.pageData(w, r, "Reset Password — rstash", &resetPasswordContent{Success: true}))
}

// --- Helpers ---

func (h *accountHandler) sendVerificationEmail(r *http.Request, userID int64, emailAddr string) {
	token := generateToken()
	expiry := time.Now().UTC().Add(48 * time.Hour)
	if err := h.deps.Repo.SetEmailVerifyToken(r.Context(), userID, token, expiry); err != nil {
		slog.Error("failed to set email verify token", "error", err)
		return
	}

	msg := email.VerificationEmail(emailAddr, h.deps.Config.BaseURL, token)
	if err := h.deps.Mailer.Send(r.Context(), msg); err != nil {
		slog.Error("failed to send verification email", "error", err)
		metrics.EmailsSentTotal.WithLabelValues("verification", "failed").Inc()
		_ = h.deps.Repo.LogEmailSend(r.Context(), emailAddr, "verification", msg.Subject, "failed", err.Error())
	} else {
		metrics.EmailsSentTotal.WithLabelValues("verification", "sent").Inc()
		_ = h.deps.Repo.LogEmailSend(r.Context(), emailAddr, "verification", msg.Subject, "sent", "")
	}
}

func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}
