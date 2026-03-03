package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"gosilo/internal/api"
	"gosilo/internal/config"
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
	Module      string
	Permission  string
	Description string
	Raw         string
	IsWrite     bool
	BadgeText   string
}

type authorizeContent struct {
	ClientID            string
	ClientOrigin        string
	RedirectURI         string
	Username            string
	LoggedIn            bool
	LoginURL            string
	Scopes              []string
	ScopeDisplay        []scopeDisplay
	HasRootScope        bool
	State               string
	ResponseType        string
	CodeChallenge       string
	CodeChallengeMethod string
	Error               string
	TOSUrl              string
	PrivacyUrl          string
	ClientIsURL    bool
	ClientHostname string
}

func buildScopeDisplay(scopes []string) ([]scopeDisplay, bool) {
	out := make([]scopeDisplay, 0, len(scopes))
	hasRoot := false
	for _, s := range scopes {
		parts := strings.SplitN(s, ":", 2)
		module := parts[0]
		access := ""
		if len(parts) > 1 {
			access = parts[1]
		}

		isWrite := access == "rw"
		display := scopeDisplay{Raw: s, IsWrite: isWrite}
		if isWrite {
			display.BadgeText = "Read & write"
		} else {
			display.BadgeText = "Read only"
		}
		if module == "*" {
			display.Module = "All modules"
			if isWrite {
				display.Description = "Full access to all your data"
				hasRoot = true
			} else {
				display.Description = "Read all your data"
			}
		} else {
			display.Module = "/" + module
			if isWrite {
				display.Description = fmt.Sprintf("Read and write your %s data", module)
			} else {
				display.Description = fmt.Sprintf("Read your %s data", module)
			}
		}
		if isWrite {
			display.Permission = "read & write"
		} else {
			display.Permission = "read only"
		}
		out = append(out, display)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].IsWrite && !out[j].IsWrite
	})
	return out, hasRoot
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

	scopeDisplayList, hasRootScope := buildScopeDisplay(scopes)

	// Client identity is always the redirect_uri origin per RS spec
	// (draft-dejong-remotestorage-26: server MUST ignore client_id param).
	var clientIsURL bool
	var clientHostname string
	if u, err := url.Parse(origin); err == nil && u.Scheme != "" && u.Host != "" {
		clientIsURL = true
		clientHostname = u.Hostname()
	}

	snap := h.deps.Settings.Load()
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

	h.deps.Renderer.Render(w, "oauth_authorize", ui.PageData{
		Title:       "Authorize — Gosilo",
		CurrentUser: userInfo(user),
		CSRFToken:   CSRFToken(r),
		Content: &authorizeContent{
			ClientID:            origin,
			ClientOrigin:        origin,
			RedirectURI:         redirectURI,
			Username:            username,
			LoggedIn:            loggedIn,
			LoginURL:            loginURL,
			Scopes:              scopes,
			ScopeDisplay:        scopeDisplayList,
			HasRootScope:        hasRootScope,
			State:               state,
			ResponseType:        responseType,
			CodeChallenge:       codeChallenge,
			CodeChallengeMethod: codeChallengeMethod,
			TOSUrl:              tosUrl,
			PrivacyUrl:          privacyUrl,
			ClientIsURL:    clientIsURL,
			ClientHostname: clientHostname,
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
	_, err = h.deps.Repo.UpsertOAuthClient(r.Context(), origin, redirectURI)
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

		ac, err := h.deps.Repo.CreateAuthorizationCode(r.Context(), user.ID, origin, redirectURI, scopeStr, codeChallenge, codeChallengeMethod)
		if err != nil {
			slog.Error("create authorization code", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		h.deps.Repo.Audit(r.Context(), user.ID, "oauth.token_granted", "oauth_client", origin, fmt.Sprintf("code flow, scopes: %s", scopeStr))

		q := redirectBase.Query()
		q.Set("code", ac.Code)
		if state != "" {
			q.Set("state", state)
		}
		redirectBase.RawQuery = q.Encode()
		http.Redirect(w, r, redirectBase.String(), http.StatusFound)
	} else {
		// Implicit flow: create token and redirect with fragment.
		lifetimeStr := h.deps.Settings.Load().TokenLifetime
		lifetime, _ := config.ParseTokenLifetime(lifetimeStr)

		token, err := h.deps.Repo.CreateOAuthToken(r.Context(), user.ID, origin, validScopes, lifetime)
		if err != nil {
			slog.Error("create oauth token", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}

		h.deps.Repo.Audit(r.Context(), user.ID, "oauth.token_granted", "oauth_client", origin, fmt.Sprintf("implicit flow, scopes: %s", scopeStr))

		fragment := "access_token=" + url.QueryEscape(token.Token) + "&token_type=bearer"
		if lifetime > 0 {
			fragment += "&expires_in=" + fmt.Sprintf("%d", int64(lifetime.Seconds()))
		}
		if state != "" {
			fragment += "&state=" + url.QueryEscape(state)
		}
		redirectBase.Fragment = fragment
		http.Redirect(w, r, redirectBase.String(), http.StatusFound)
	}
}
