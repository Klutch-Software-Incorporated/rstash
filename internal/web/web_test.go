package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"rstash/internal/auth"
	"rstash/internal/config"
	"rstash/internal/db"
	"rstash/internal/settings"
	"rstash/internal/ui"
	"rstash/internal/web"
)

func timeNowPlus(d time.Duration) time.Time {
	return time.Now().UTC().Add(d)
}

func setupTestServer(t *testing.T, regMode string) (*httptest.Server, *web.UIDeps) {
	t.Helper()

	repo, err := db.OpenRepository("sqlite::memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	renderer := ui.NewRenderer()
	cfg := &config.Config{
		Addr:             ":8080",
		BaseURL:          "http://localhost:8080",
		DatabaseDSN:      "sqlite::memory:",
		BlobDSN:          "sqlite::memory:",
		RegistrationMode: regMode,
		LogLevel:         "error",
	}

	localAuth := auth.NewLocalService(repo)
	runtimeSettings := settings.New(repo, cfg)

	deps := &web.UIDeps{
		Auth:     localAuth,
		Repo:     repo,
		Renderer: renderer,
		Config:   cfg,
		Settings: runtimeSettings,
	}

	uiHandler := web.FullRoutes(deps)
	wrapped := web.AuthLoader(localAuth, runtimeSettings, false)(
		web.EnsureCSRFCookie(runtimeSettings, false)(
			web.SetupGuard(localAuth)(uiHandler),
		),
	)

	ts := httptest.NewServer(wrapped)
	t.Cleanup(ts.Close)

	return ts, deps
}

func TestSetupRedirectWhenNoUsers(t *testing.T) {
	ts, _ := setupTestServer(t, "closed")

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("get /: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/setup" {
		t.Fatalf("expected redirect to /setup, got %s", loc)
	}
}

func TestSetupCreatesAdmin(t *testing.T) {
	ts, _ := setupTestServer(t, "closed")

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	// GET /setup should render the review page.
	resp, err := client.Get(ts.URL + "/setup")
	if err != nil {
		t.Fatalf("get /setup: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for GET /setup, got %d", resp.StatusCode)
	}

	// GET /setup?step=account should render the account form.
	resp, err = client.Get(ts.URL + "/setup?step=account")
	if err != nil {
		t.Fatalf("get /setup?step=account: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for GET /setup?step=account, got %d", resp.StatusCode)
	}

	// Extract CSRF token from the GET response cookies.
	var csrfToken string
	for _, c := range resp.Cookies() {
		if c.Name == "rstash_csrf" {
			csrfToken = c.Value
		}
	}
	if csrfToken == "" {
		t.Fatal("expected rstash_csrf cookie from GET /setup?step=account")
	}

	// POST /setup to create admin.
	form := url.Values{
		"username":         {"admin"},
		"email":            {"admin@example.com"},
		"password":         {"secretpassword"},
		"password_confirm": {"secretpassword"},
	}
	resp = postWithCSRF(t, client, ts.URL+"/setup", csrfToken, form)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after setup, got %d", resp.StatusCode)
	}

	// Should have a session cookie.
	var hasCookie bool
	for _, c := range resp.Cookies() {
		if c.Name == "rstash_session" && c.Value != "" {
			hasCookie = true
		}
	}
	if !hasCookie {
		t.Fatal("expected rstash_session cookie after setup")
	}
}

func TestLoginLogoutCycle(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	// Create a user directly.
	_, err := deps.Repo.CreateUser(context.Background(), "testuser", "password123", "", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	jar := &simpleCookieJar{cookies: make(map[string][]*http.Cookie)}
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Jar: jar,
	}

	// Get CSRF token.
	csrfToken := getCSRFToken(t, client, ts.URL+"/login")

	// Login.
	form := url.Values{
		"username": {"testuser"},
		"password": {"password123"},
	}
	resp := postWithCSRF(t, client, ts.URL+"/login", csrfToken, form)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 after login, got %d", resp.StatusCode)
	}

	// Verify we have a session cookie.
	var sessionToken string
	for _, c := range resp.Cookies() {
		if c.Name == "rstash_session" {
			sessionToken = c.Value
		}
	}
	if sessionToken == "" {
		t.Fatal("expected session cookie after login")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	_, _ = deps.Repo.CreateUser(context.Background(), "badloginuser", "password123", "", false, true)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	csrfToken := getCSRFToken(t, client, ts.URL+"/login")

	form := url.Values{
		"username": {"badloginuser"},
		"password": {"wrongpassword"},
	}
	resp := postWithCSRF(t, client, ts.URL+"/login", csrfToken, form)
	resp.Body.Close()

	// Should re-render the login page (200), not redirect.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for bad login, got %d", resp.StatusCode)
	}
}

func TestAdminRequiresAuth(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	// Create a user so setup guard doesn't redirect.
	_, _ = deps.Repo.CreateUser(context.Background(), "adminauthuser", "password123", "", true, true)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(ts.URL + "/admin")
	if err != nil {
		t.Fatalf("get /admin: %v", err)
	}
	resp.Body.Close()

	// Should redirect to login.
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect to login, got %d", resp.StatusCode)
	}
}

func TestRegistrationClosed(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	_, _ = deps.Repo.CreateUser(context.Background(), "regcloseduser", "password123", "", false, true)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := client.Get(ts.URL + "/register")
	if err != nil {
		t.Fatalf("get /register: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRegistrationClosedPostReturns403(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	_, _ = deps.Repo.CreateUser(context.Background(), "closedpostuser", "password123", "", false, true)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	form := url.Values{
		"username":         {"newuser"},
		"password":         {"newpassword123"},
		"password_confirm": {"newpassword123"},
	}
	resp, err := client.Post(ts.URL+"/register", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("post /register: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for closed registration POST, got %d", resp.StatusCode)
	}
}

func TestRegistrationOpen(t *testing.T) {
	ts, deps := setupTestServer(t, "open")

	_, _ = deps.Repo.CreateUser(context.Background(), "openreguser", "password123", "", false, true)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	csrfToken := getCSRFToken(t, client, ts.URL+"/register")

	form := url.Values{
		"username":         {"newuser"},
		"email":            {"newuser@example.com"},
		"password":         {"newpassword123"},
		"password_confirm": {"newpassword123"},
		"tos_accept":       {"on"},
	}
	resp := postWithCSRF(t, client, ts.URL+"/register", csrfToken, form)
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 after registration, got %d", resp.StatusCode)
	}

	var hasCookie bool
	for _, c := range resp.Cookies() {
		if c.Name == "rstash_session" && c.Value != "" {
			hasCookie = true
		}
	}
	if !hasCookie {
		t.Fatal("expected session cookie after registration")
	}
}

func TestSetupValidation(t *testing.T) {
	ts, _ := setupTestServer(t, "closed")

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	csrfToken := getCSRFToken(t, client, ts.URL+"/setup?step=account")

	// Password too short.
	form := url.Values{
		"username":         {"admin"},
		"password":         {"short"},
		"password_confirm": {"short"},
	}
	resp := postWithCSRF(t, client, ts.URL+"/setup", csrfToken, form)
	resp.Body.Close()

	// Should re-render form (200), not redirect.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for validation error, got %d", resp.StatusCode)
	}

	// Password mismatch.
	form = url.Values{
		"username":         {"admin"},
		"password":         {"longpassword123"},
		"password_confirm": {"differentpassword"},
	}
	resp = postWithCSRF(t, client, ts.URL+"/setup", csrfToken, form)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for password mismatch, got %d", resp.StatusCode)
	}
}

func TestExternalRegistrationRedirects(t *testing.T) {
	ts, deps := setupTestServer(t, "external")

	// Ensure at least one user exists so SetupGuard lets us through to /register.
	_, err := deps.Repo.CreateUser(context.Background(), "admin", "password123", "", true, true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := deps.Settings.Set(context.Background(), "registration_external_url", "https://example.com/signup"); err != nil {
		t.Fatalf("set url: %v", err)
	}

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/register")
	if err != nil {
		t.Fatalf("get /register: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "https://example.com/signup" {
		t.Fatalf("expected redirect to https://example.com/signup, got %q", loc)
	}
}

func TestClaimFlow(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	// Simulate an admin-API-provisioned user.
	user, err := deps.Repo.CreateUser(context.Background(), "newuser", "placeholderpwd1234567", "new@example.com", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	_ = deps.Repo.SetExternallyManaged(context.Background(), user.ID, true)
	token := "claim-token-abc123"
	if err := deps.Repo.SetPasswordResetToken(context.Background(), user.ID, token, timeNowPlus(24*time.Hour)); err != nil {
		t.Fatalf("set token: %v", err)
	}

	jar := &simpleCookieJar{cookies: make(map[string][]*http.Cookie)}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// GET /claim with the token should succeed and set a CSRF cookie.
	resp, err := client.Get(ts.URL + "/claim?token=" + token)
	if err != nil {
		t.Fatalf("get /claim: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for GET /claim, got %d", resp.StatusCode)
	}

	var csrfToken string
	for _, c := range resp.Cookies() {
		if c.Name == "rstash_csrf" {
			csrfToken = c.Value
		}
	}
	if csrfToken == "" {
		t.Fatal("expected rstash_csrf cookie")
	}

	// POST /claim with new password.
	form := url.Values{
		"token":            {token},
		"password":         {"newpassword123"},
		"password_confirm": {"newpassword123"},
	}
	resp = postWithCSRF(t, client, ts.URL+"/claim", csrfToken, form)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 after claim, got %d", resp.StatusCode)
	}

	// Session cookie should be set.
	var hasSession bool
	for _, c := range resp.Cookies() {
		if c.Name == "rstash_session" && c.Value != "" {
			hasSession = true
		}
	}
	if !hasSession {
		t.Fatal("expected rstash_session cookie after claim")
	}

	// User's claim token should be cleared and they can log in with new password.
	updated, err := deps.Repo.GetUserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.PasswordResetToken != nil && *updated.PasswordResetToken != "" {
		t.Error("expected claim token to be cleared after claim")
	}
}

func TestClaimFlow_InvalidToken(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")
	// Need a user to exist so SetupGuard doesn't redirect to /setup.
	_, err := deps.Repo.CreateUser(context.Background(), "admin", "password123", "", true, true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/claim?token=does-not-exist")
	if err != nil {
		t.Fatalf("get /claim: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (error page), got %d", resp.StatusCode)
	}
}

func TestExternalRegistrationWithoutURL(t *testing.T) {
	ts, deps := setupTestServer(t, "external")
	_, err := deps.Repo.CreateUser(context.Background(), "admin", "password123", "", true, true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(ts.URL + "/register")
	if err != nil {
		t.Fatalf("get /register: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when URL unset, got %d", resp.StatusCode)
	}
}

// getCSRFToken performs a GET request to the given URL and returns the CSRF
// cookie value. Tests must include this value as the csrf_token form field.
func getCSRFToken(t *testing.T, client *http.Client, url string) string {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("get csrf token: %v", err)
	}
	resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "rstash_csrf" {
			return c.Value
		}
	}
	t.Fatal("no rstash_csrf cookie in response")
	return ""
}

// postWithCSRF performs a POST request that includes the CSRF cookie and form token.
func postWithCSRF(t *testing.T, client *http.Client, targetURL string, csrfToken string, form url.Values) *http.Response {
	t.Helper()
	form.Set("csrf_token", csrfToken)
	req, err := http.NewRequest("POST", targetURL, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "rstash_csrf", Value: csrfToken})
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post with csrf: %v", err)
	}
	return resp
}

// simpleCookieJar is a minimal cookie jar for testing.
type simpleCookieJar struct {
	cookies map[string][]*http.Cookie
}

func (j *simpleCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.cookies[u.Host] = append(j.cookies[u.Host], cookies...)
}

func (j *simpleCookieJar) Cookies(u *url.URL) []*http.Cookie {
	return j.cookies[u.Host]
}
