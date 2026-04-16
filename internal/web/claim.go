package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"rstash/internal/auth"
	"rstash/internal/ui"
)

type claimHandler struct {
	deps *UIDeps
}

// ClaimHandler returns handler methods for the /claim account-claim flow,
// used by users who were provisioned via the admin API and need to set
// their password before their first login.
func ClaimHandler(deps *UIDeps) *claimHandler {
	return &claimHandler{deps: deps}
}

type claimContent struct {
	Token    string
	Email    string
	Username string
	Error    string
	Success  bool
}

// ShowClaim handles GET /claim?token=...
func (h *claimHandler) ShowClaim(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		h.deps.Renderer.Render(w, "claim", h.deps.pageData(w, r, "Activate Account — rstash", &claimContent{Error: "Missing claim token."}))
		return
	}

	user, err := h.deps.Repo.GetUserByPasswordResetToken(r.Context(), token)
	if err != nil {
		slog.Error("claim: look up token", "error", err)
		h.deps.Renderer.Render(w, "claim", h.deps.pageData(w, r, "Activate Account — rstash", &claimContent{Error: "An error occurred."}))
		return
	}
	if user == nil {
		h.deps.Repo.Audit(r.Context(), 0, "user.claim_token_expired", "claim", "", "")
		h.deps.Renderer.Render(w, "claim", h.deps.pageData(w, r, "Activate Account — rstash", &claimContent{Error: "Invalid or expired activation link. Ask your operator to issue a new one."}))
		return
	}

	content := &claimContent{Token: token, Username: user.Username}
	if user.Email != nil {
		content.Email = *user.Email
	}
	h.deps.Renderer.Render(w, "claim", h.deps.pageData(w, r, "Activate Account — rstash", content))
}

// DoClaim handles POST /claim
func (h *claimHandler) DoClaim(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "claim", h.deps.pageData(w, r, "Activate Account — rstash", &claimContent{Token: token, Error: msg}))
	}

	if token == "" {
		renderErr("Missing claim token.")
		return
	}

	user, err := h.deps.Repo.GetUserByPasswordResetToken(r.Context(), token)
	if err != nil {
		slog.Error("claim: look up token", "error", err)
		renderErr("An error occurred.")
		return
	}
	if user == nil {
		renderErr("Invalid or expired activation link.")
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
		slog.Error("claim: update password", "error", err)
		renderErr("An error occurred. Please try again.")
		return
	}

	_ = h.deps.Repo.ClearPasswordResetToken(r.Context(), user.ID)

	// Create a session and log the user in.
	sess, err := h.deps.Auth.CreateSession(r.Context(), user.ID, h.deps.ClientIPForStorage(r))
	if err != nil {
		slog.Error("claim: create session", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	_ = h.deps.Repo.UpdateUserLastLogin(r.Context(), user.ID, h.deps.ClientIPForStorage(r))
	h.deps.Repo.Audit(r.Context(), user.ID, "user.claimed", "user", fmt.Sprintf("%d", user.ID), user.Username)
	if h.deps.Webhooks != nil {
		claimedAt := time.Now().UTC().Format(time.RFC3339)
		data := map[string]any{"username": user.Username, "claimed_at": claimedAt}
		if user.Email != nil {
			data["email"] = *user.Email
		}
		h.deps.Webhooks.Emit(r.Context(), "user.claimed", data)
	}
	auth.SetSessionCookie(w, sess.Token, h.deps.Settings.Load().CookieDomain, h.deps.SecureCookies)
	ui.SetFlash(w, "Welcome! Your account is activated.")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
