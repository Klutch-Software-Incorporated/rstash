package api_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gosilo/internal/api"
	"gosilo/internal/db"
)

func tokenTestServer(t *testing.T) (*httptest.Server, *tokenTestEnv) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	handler := api.OAuthToken(database, func() string { return "30d" }, func() (bool, string) { return false, "" })
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	ctx := context.Background()
	user, err := db.CreateUser(ctx, database, "tokenuser", "password", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = db.UpsertOAuthClient(ctx, database, "https://app.example.com", "https://app.example.com/callback")
	if err != nil {
		t.Fatalf("upsert client: %v", err)
	}

	return ts, &tokenTestEnv{
		DB:     database,
		UserID: user.ID,
	}
}

type tokenTestEnv struct {
	DB     db.Querier
	UserID int64
}

func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func createAuthCode(t *testing.T, dbq db.Querier, userID int64, verifier string) string {
	t.Helper()
	challenge := pkceChallenge(verifier)
	ac, err := db.CreateAuthorizationCode(
		context.Background(), dbq,

		userID, "https://app.example.com", "https://app.example.com/callback",
		"*:rw", challenge, "S256",
	)
	if err != nil {
		t.Fatalf("create auth code: %v", err)
	}
	return ac.Code
}

func TestTokenExchange_ValidPKCE(t *testing.T) {
	ts, env := tokenTestServer(t)
	client := ts.Client()

	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	code := createAuthCode(t, env.DB, env.UserID, verifier)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {"https://app.example.com/callback"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["access_token"] == nil || result["access_token"] == "" {
		t.Fatal("expected access_token in response")
	}
	if result["token_type"] != "bearer" {
		t.Fatalf("expected token_type bearer, got %v", result["token_type"])
	}
	if result["expires_in"] == nil {
		t.Fatal("expected expires_in in response (token_lifetime=30d)")
	}

	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected Cache-Control: no-store, got %q", cc)
	}
}

func TestTokenExchange_InvalidVerifier(t *testing.T) {
	ts, env := tokenTestServer(t)
	client := ts.Client()

	verifier := "correct-verifier-value-for-testing"
	code := createAuthCode(t, env.DB, env.UserID, verifier)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {"wrong-verifier-value"},
		"redirect_uri":  {"https://app.example.com/callback"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid verifier, got %d", resp.StatusCode)
	}

	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] != "invalid_grant" {
		t.Fatalf("expected error=invalid_grant, got %q", result["error"])
	}
}

func TestTokenExchange_InvalidCode(t *testing.T) {
	ts, _ := tokenTestServer(t)
	client := ts.Client()

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {"nonexistent-code"},
		"code_verifier": {"some-verifier"},
		"redirect_uri":  {"https://app.example.com/callback"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid code, got %d", resp.StatusCode)
	}
}

func TestTokenExchange_WrongRedirectURI(t *testing.T) {
	ts, env := tokenTestServer(t)
	client := ts.Client()

	verifier := "test-verifier-for-redirect"
	code := createAuthCode(t, env.DB, env.UserID, verifier)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {"https://evil.example.com/callback"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for wrong redirect_uri, got %d", resp.StatusCode)
	}
}

func TestTokenExchange_MissingParams(t *testing.T) {
	ts, _ := tokenTestServer(t)
	client := ts.Client()

	form := url.Values{
		"grant_type": {"authorization_code"},
		// Missing code, code_verifier, redirect_uri.
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing params, got %d", resp.StatusCode)
	}
}

func TestTokenExchange_UnsupportedGrantType(t *testing.T) {
	ts, _ := tokenTestServer(t)
	client := ts.Client()

	form := url.Values{
		"grant_type": {"client_credentials"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported grant type, got %d", resp.StatusCode)
	}
}

func TestTokenExchange_GetNotAllowed(t *testing.T) {
	ts, _ := tokenTestServer(t)
	client := ts.Client()

	resp, err := client.Get(ts.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for GET, got %d", resp.StatusCode)
	}
}

func TestTokenExchange_CodeReuse(t *testing.T) {
	ts, env := tokenTestServer(t)
	client := ts.Client()

	verifier := "verifier-for-reuse-test"
	code := createAuthCode(t, env.DB, env.UserID, verifier)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {"https://app.example.com/callback"},
	}

	// First use: should succeed.
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first use: expected 200, got %d", resp.StatusCode)
	}

	// Second use: should fail (code already used).
	resp, err = client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("second use: expected 400 for reused code, got %d", resp.StatusCode)
	}
}

func refreshTokenTestServer(t *testing.T) (*httptest.Server, *tokenTestEnv) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	handler := api.OAuthToken(database, func() string { return "30d" }, func() (bool, string) { return true, "90d" })
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	ctx := context.Background()
	user, err := db.CreateUser(ctx, database, "refreshuser", "password", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = db.UpsertOAuthClient(ctx, database, "https://app.example.com", "https://app.example.com/callback")
	if err != nil {
		t.Fatalf("upsert client: %v", err)
	}

	return ts, &tokenTestEnv{
		DB:     database,
		UserID: user.ID,
	}
}

func TestTokenExchange_RefreshTokenIssued(t *testing.T) {
	ts, env := refreshTokenTestServer(t)
	client := ts.Client()

	verifier := "refresh-test-verifier"
	code := createAuthCode(t, env.DB, env.UserID, verifier)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {"https://app.example.com/callback"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["refresh_token"] == nil || result["refresh_token"] == "" {
		t.Fatal("expected refresh_token in response when refresh tokens are enabled")
	}
}

func TestTokenExchange_RefreshTokenGrant(t *testing.T) {
	ts, env := refreshTokenTestServer(t)
	client := ts.Client()

	// Step 1: Get an initial access + refresh token pair.
	verifier := "refresh-grant-verifier"
	code := createAuthCode(t, env.DB, env.UserID, verifier)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {"https://app.example.com/callback"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	var result1 map[string]any
	json.NewDecoder(resp.Body).Decode(&result1)
	refreshToken := result1["refresh_token"].(string)
	oldAccessToken := result1["access_token"].(string)

	// Step 2: Exchange refresh token for new tokens.
	form2 := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	resp2, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form2.Encode()))
	if err != nil {
		t.Fatalf("POST refresh: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for refresh, got %d", resp2.StatusCode)
	}

	var result2 map[string]any
	json.NewDecoder(resp2.Body).Decode(&result2)

	newAccessToken := result2["access_token"].(string)
	newRefreshToken := result2["refresh_token"]

	if newAccessToken == "" {
		t.Fatal("expected new access_token")
	}
	if newAccessToken == oldAccessToken {
		t.Fatal("expected a different access_token after refresh")
	}
	if newRefreshToken == nil || newRefreshToken == "" {
		t.Fatal("expected new refresh_token (rotation)")
	}
	if newRefreshToken == refreshToken {
		t.Fatal("expected rotated refresh_token (different from old)")
	}

	// Step 3: Old refresh token should no longer work.
	resp3, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form2.Encode()))
	if err != nil {
		t.Fatalf("POST old refresh: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for reused refresh token, got %d", resp3.StatusCode)
	}
}

func TestTokenExchange_RefreshTokenDisabled(t *testing.T) {
	ts, env := tokenTestServer(t) // uses disabled refresh
	client := ts.Client()

	verifier := "no-refresh-verifier"
	code := createAuthCode(t, env.DB, env.UserID, verifier)

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {"https://app.example.com/callback"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	if result["refresh_token"] != nil {
		t.Fatal("expected no refresh_token when refresh tokens are disabled")
	}
}
