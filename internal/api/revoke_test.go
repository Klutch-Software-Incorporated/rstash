package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"gosilo/internal/api"
	"gosilo/internal/db"
)

func revokeTestServer(t *testing.T) (*httptest.Server, *revokeTestEnv) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	handler := api.CORS(api.OAuthRevoke(database))
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	ctx := context.Background()
	user, err := db.CreateUser(ctx, database, "revokeuser", "password", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = db.UpsertOAuthClient(ctx, database, "https://app.example.com", "https://app.example.com/callback")
	if err != nil {
		t.Fatalf("upsert client: %v", err)
	}

	return ts, &revokeTestEnv{
		DB:     database,
		UserID: user.ID,
	}
}

type revokeTestEnv struct {
	DB     db.Querier
	UserID int64
}

func TestRevoke_AccessToken(t *testing.T) {
	ts, env := revokeTestServer(t)
	client := ts.Client()
	ctx := context.Background()

	// Create an access token.
	token, err := db.CreateOAuthToken(ctx, env.DB, env.UserID, "https://app.example.com", []string{"*:rw"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	// Also create a linked refresh token.
	_, err = db.CreateRefreshToken(ctx, env.DB, env.UserID, "https://app.example.com", []string{"*:rw"}, token.Token, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	// Revoke the access token.
	form := url.Values{"token": {token.Token}}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify access token is deleted.
	got, _ := db.GetOAuthToken(ctx, env.DB, token.Token)
	if got != nil {
		t.Fatal("expected access token to be deleted")
	}
}

func TestRevoke_RefreshToken(t *testing.T) {
	ts, env := revokeTestServer(t)
	client := ts.Client()
	ctx := context.Background()

	// Create an access token and linked refresh token.
	accessToken, err := db.CreateOAuthToken(ctx, env.DB, env.UserID, "https://app.example.com", []string{"*:rw"}, 24*time.Hour)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	rt, err := db.CreateRefreshToken(ctx, env.DB, env.UserID, "https://app.example.com", []string{"*:rw"}, accessToken.Token, 90*24*time.Hour)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	// Revoke the refresh token with hint.
	form := url.Values{
		"token":           {rt.Token},
		"token_type_hint": {"refresh_token"},
	}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify both tokens are deleted (cascade).
	gotRT, _ := db.GetRefreshToken(ctx, env.DB, rt.Token)
	if gotRT != nil {
		t.Fatal("expected refresh token to be deleted")
	}

	gotAT, _ := db.GetOAuthToken(ctx, env.DB, accessToken.Token)
	if gotAT != nil {
		t.Fatal("expected access token to be cascade-deleted")
	}
}

func TestRevoke_UnknownToken(t *testing.T) {
	ts, _ := revokeTestServer(t)
	client := ts.Client()

	form := url.Values{"token": {"nonexistent-token-value-12345678"}}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	// Per RFC 7009, always 200 OK even for unknown tokens.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestRevoke_MissingParam(t *testing.T) {
	ts, _ := revokeTestServer(t)
	client := ts.Client()

	form := url.Values{}
	resp, err := client.Post(ts.URL, "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
