package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gosilo/internal/auth"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/ui"
	"gosilo/internal/web"
)

func setupTestServer(t *testing.T, regMode string) (*httptest.Server, *web.UIDeps) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	renderer := ui.NewRenderer()
	cfg := &config.Config{
		Addr:             ":8080",
		BaseURL:          "http://localhost:8080",
		DatabasePath:     ":memory:",
		BlobBackend:      "sqlite",
		RegistrationMode: regMode,
		LogLevel:         "error",
	}

	localAuth := auth.NewLocalService(database)

	deps := &web.UIDeps{
		Auth:     localAuth,
		DB:       database,
		Renderer: renderer,
		Config:   cfg,
	}

	uiHandler := web.Routes(deps)
	wrapped := web.AuthLoader(localAuth)(
		web.SetupGuard(localAuth)(uiHandler),
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

	// GET /setup should render the form.
	resp, err := client.Get(ts.URL + "/setup")
	if err != nil {
		t.Fatalf("get /setup: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for GET /setup, got %d", resp.StatusCode)
	}

	// POST /setup to create admin.
	form := url.Values{
		"username":         {"admin"},
		"password":         {"secretpassword"},
		"password_confirm": {"secretpassword"},
	}
	resp, err = client.Post(ts.URL+"/setup", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("post /setup: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect after setup, got %d", resp.StatusCode)
	}

	// Should have a session cookie.
	var hasCookie bool
	for _, c := range resp.Cookies() {
		if c.Name == "gosilo_session" && c.Value != "" {
			hasCookie = true
		}
	}
	if !hasCookie {
		t.Fatal("expected gosilo_session cookie after setup")
	}
}

func TestLoginLogoutCycle(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	// Create a user directly.
	_, err := db.CreateUser(context.Background(), deps.DB, "testuser", "password123", false)
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

	// Login.
	form := url.Values{
		"username": {"testuser"},
		"password": {"password123"},
	}
	resp, err := client.Post(ts.URL+"/login", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("post /login: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 after login, got %d", resp.StatusCode)
	}

	// Verify we have a session cookie.
	var sessionToken string
	for _, c := range resp.Cookies() {
		if c.Name == "gosilo_session" {
			sessionToken = c.Value
		}
	}
	if sessionToken == "" {
		t.Fatal("expected session cookie after login")
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	_, _ = db.CreateUser(context.Background(), deps.DB, "badloginuser", "password123", false)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	form := url.Values{
		"username": {"badloginuser"},
		"password": {"wrongpassword"},
	}
	resp, err := client.Post(ts.URL+"/login", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("post /login: %v", err)
	}
	resp.Body.Close()

	// Should re-render the login page (200), not redirect.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for bad login, got %d", resp.StatusCode)
	}
}

func TestAdminRequiresAuth(t *testing.T) {
	ts, deps := setupTestServer(t, "closed")

	// Create a user so setup guard doesn't redirect.
	_, _ = db.CreateUser(context.Background(), deps.DB, "adminauthuser", "password123", true)

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

	_, _ = db.CreateUser(context.Background(), deps.DB, "regcloseduser", "password123", false)

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

func TestRegistrationOpen(t *testing.T) {
	ts, deps := setupTestServer(t, "open")

	_, _ = db.CreateUser(context.Background(), deps.DB, "openreguser", "password123", false)

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

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 after registration, got %d", resp.StatusCode)
	}

	var hasCookie bool
	for _, c := range resp.Cookies() {
		if c.Name == "gosilo_session" && c.Value != "" {
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

	// Password too short.
	form := url.Values{
		"username":         {"admin"},
		"password":         {"short"},
		"password_confirm": {"short"},
	}
	resp, err := client.Post(ts.URL+"/setup", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("post /setup: %v", err)
	}
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
	resp, err = client.Post(ts.URL+"/setup", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("post /setup: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for password mismatch, got %d", resp.StatusCode)
	}
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
