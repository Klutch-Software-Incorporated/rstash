package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rstash/internal/auth"
)

func TestAdminAPI_Me_NoCookie(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	req := httptest.NewRequest("GET", "/api/admin/me", nil)
	req.Header.Set("X-API-Key", "test-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session cookie, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAPI_Me_InvalidSession(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	req := httptest.NewRequest("GET", "/api/admin/me", nil)
	req.Header.Set("X-API-Key", "test-key")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "nonexistent-token"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid session, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAPI_Me_Success(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	email := "alice@example.com"
	user, err := repo.CreateUser(t.Context(), "alice", "password123", email, false, true)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := repo.CreateSession(t.Context(), user.ID, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/admin/me", nil)
	req.Header.Set("X-API-Key", "test-key")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sess.Token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Data meResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Data.PreferredUsername != "alice" {
		t.Errorf("expected username alice, got %q", body.Data.PreferredUsername)
	}
	if body.Data.Email == nil || *body.Data.Email != email {
		t.Errorf("expected email %q, got %v", email, body.Data.Email)
	}
	if body.Data.AccountState != "active" {
		t.Errorf("expected account_state active, got %q", body.Data.AccountState)
	}
	if body.Data.Sub == "" {
		t.Error("expected sub to be non-empty")
	}
}

func TestAdminAPI_Me_DisabledUser(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	user, err := repo.CreateUser(t.Context(), "bob", "password123", "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateUserDisabled(t.Context(), user.ID, true); err != nil {
		t.Fatal(err)
	}
	sess, err := repo.CreateSession(t.Context(), user.ID, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/admin/me", nil)
	req.Header.Set("X-API-Key", "test-key")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sess.Token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var body struct {
		Data meResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Data.AccountState != "disabled" {
		t.Errorf("expected account_state disabled, got %q", body.Data.AccountState)
	}
}
