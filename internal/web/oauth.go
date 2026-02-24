package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"gosilo/internal/api"
	"gosilo/internal/db"
	"gosilo/internal/ui"
)

type oauthHandler struct {
	deps *UIDeps
}

// OAuthHandler returns handler methods for OAuth authorization.
func OAuthHandler(deps *UIDeps) *oauthHandler {
	return &oauthHandler{deps: deps}
}

type scopeDisplay struct {
	Module     string
	Permission string
	Raw        string
}

type authorizeContent struct {
	ClientID            string
	RedirectURI         string
	Username            string
	LoggedIn            bool
	LoginURL            string
	Scopes              []string
	ScopeDisplay        []scopeDisplay
	State               string
	ResponseType        string
	CodeChallenge       string
	CodeChallengeMethod string
	Error               string
}

func buildScopeDisplay(scopes []string) []scopeDisplay {
	out := make([]scopeDisplay, 0, len(scopes))
	for _, s := range scopes {
		parts := strings.SplitN(s, ":", 2)
		module := parts[0]
		access := ""
		if len(parts) > 1 {
			access = parts[1]
		}

		display := scopeDisplay{Raw: s}
		if module == "*" {
			display.Module = "All modules"
		} else {
			display.Module = "/" + module
		}
		if access == "rw" {
			display.Permission = "read & write"
		} else {
			display.Permission = "read only"
		}
		out = append(out, display)
	}
	return out
}

// extractOrigin returns the scheme + host of a URL (the effective client ID per RS spec).
func extractOrigin(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("missing scheme or host")
	}
	return u.Scheme + "://" + u.Host, nil
}

// ShowAuthorize handles GET /oauth/authorize.
func (h *oauthHandler) ShowAuthorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	responseType := q.Get("response_type")
	if responseType != "token" && responseType != "code" {
		http.Error(w, "unsupported response_type: must be 'token' or 'code'", http.StatusBadRequest)
		return
	}

	redirectURI := q.Get("redirect_uri")
	if redirectURI == "" {
		http.Error(w, "missing redirect_uri", http.StatusBadRequest)
		return
	}

	origin, err := extractOrigin(redirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}

	scopeStr := q.Get("scope")
	scopes, ok := api.ParseScopes(scopeStr)
	if !ok {
		http.Error(w, "invalid scope", http.StatusBadRequest)
		return
	}

	// Code flow requires PKCE parameters.
	var codeChallenge, codeChallengeMethod string
	if responseType == "code" {
		codeChallenge = q.Get("code_challenge")
		codeChallengeMethod = q.Get("code_challenge_method")
		if codeChallenge == "" || codeChallengeMethod != "S256" {
			http.Error(w, "code flow requires code_challenge and code_challenge_method=S256", http.StatusBadRequest)
			return
		}
	}

	state := q.Get("state")

	user := CurrentUser(r)
	loggedIn := user != nil

	username := ""
	if loggedIn {
		username = user.Username
	}

	// Build login URL that returns here after authentication.
	loginURL := "/login?redirect=" + url.QueryEscape("/oauth/authorize?"+r.URL.RawQuery)

	h.deps.Renderer.Render(w, "oauth_authorize", ui.PageData{
		Title:       "Authorize — Gosilo",
		CurrentUser: userInfo(user),
		CSRFToken:   CSRFToken(r),
		Content: &authorizeContent{
			ClientID:            origin,
			RedirectURI:         redirectURI,
			Username:            username,
			LoggedIn:            loggedIn,
			LoginURL:            loginURL,
			Scopes:              scopes,
			ScopeDisplay:        buildScopeDisplay(scopes),
			State:               state,
			ResponseType:        responseType,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
		},
	})
}

// DoAuthorize handles POST /oauth/authorize.
func (h *oauthHandler) DoAuthorize(w http.ResponseWriter, r *http.Request) {
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")
	action := r.FormValue("action")
	responseType := r.FormValue("response_type")
	scopes := r.Form["scope"]

	if redirectURI == "" {
		http.Error(w, "missing redirect_uri", http.StatusBadRequest)
		return
	}

	redirectBase, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}

	isCodeFlow := responseType == "code"

	if action == "deny" {
		if isCodeFlow {
			// Code flow: error in query params.
			q := redirectBase.Query()
			q.Set("error", "access_denied")
			if state != "" {
				q.Set("state", state)
			}
			redirectBase.RawQuery = q.Encode()
		} else {
			// Implicit flow: error in fragment.
			fragment := "error=access_denied"
			if state != "" {
				fragment += "&state=" + url.QueryEscape(state)
			}
			redirectBase.Fragment = fragment
		}
		http.Redirect(w, r, redirectBase.String(), http.StatusFound)
		return
	}

	// Approve flow.
	user := CurrentUser(r)
	if user == nil {
		http.Error(w, "not authenticated", http.StatusUnauthorized)
		return
	}

	origin, err := extractOrigin(redirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}

	// Re-validate scopes from form.
	scopeStr := strings.Join(scopes, " ")
	validScopes, ok := api.ParseScopes(scopeStr)
	if !ok {
		http.Error(w, "invalid scope", http.StatusBadRequest)
		return
	}

	// Upsert client.
	_, err = db.UpsertOAuthClient(r.Context(), h.deps.DB, origin, redirectURI)
	if err != nil {
		slog.Error("upsert oauth client", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if isCodeFlow {
		// Authorization code flow: create code and redirect with query params.
		codeChallenge := r.FormValue("code_challenge")
		codeChallengeMethod := r.FormValue("code_challenge_method")
		if codeChallenge == "" || codeChallengeMethod != "S256" {
			http.Error(w, "code flow requires code_challenge and code_challenge_method=S256", http.StatusBadRequest)
			return
		}

		ac, err := db.CreateAuthorizationCode(r.Context(), h.deps.DB, user.ID, origin, redirectURI, scopeStr, codeChallenge, codeChallengeMethod)
		if err != nil {
			slog.Error("create authorization code", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		q := redirectBase.Query()
		q.Set("code", ac.Code)
		if state != "" {
			q.Set("state", state)
		}
		redirectBase.RawQuery = q.Encode()
		http.Redirect(w, r, redirectBase.String(), http.StatusFound)
	} else {
		// Implicit flow: create token and redirect with fragment.
		token, err := db.CreateOAuthToken(r.Context(), h.deps.DB, user.ID, origin, validScopes)
		if err != nil {
			slog.Error("create oauth token", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		fragment := "access_token=" + url.QueryEscape(token.Token) + "&token_type=bearer"
		if state != "" {
			fragment += "&state=" + url.QueryEscape(state)
		}
		redirectBase.Fragment = fragment
		http.Redirect(w, r, redirectBase.String(), http.StatusFound)
	}
}
