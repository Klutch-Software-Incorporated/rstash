package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"rstash/internal/db"
	"rstash/internal/model"
)

const testAPIKey = "test-key"

func setupAdminTest(t *testing.T) (*AdminDeps, *db.Repository) {
	t.Helper()
	repo, err := db.OpenRepository("sqlite::memory:")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := db.HashAPIKey(testAPIKey)
	if err != nil {
		t.Fatal(err)
	}
	key := &model.APIKey{
		Name:         "test",
		KeyHash:      hash,
		KeyPrefix:    db.APIKeyPrefix(testAPIKey),
		RateLimitRPM: 60,
	}
	if err := repo.CreateAPIKey(t.Context(), key); err != nil {
		t.Fatal(err)
	}
	deps := &AdminDeps{Repo: repo}
	return deps, repo
}

func TestAdminAPI_MissingKey(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when no key provided, got %d", w.Code)
	}
}

func TestAdminAPI_WrongAPIKey(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong key, got %d", w.Code)
	}
}

func TestAdminAPI_BearerAuth(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+testAPIKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with Bearer auth, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAPI_ListUsers(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	// Create a test user.
	_, err := repo.CreateUser(t.Context(), "alice", "password123", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp listUsersResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Total < 1 {
		t.Fatal("expected at least 1 user")
	}
}

func TestAdminAPI_GetUser(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	_, err := repo.CreateUser(t.Context(), "bob", "password123", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/admin/users/bob", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAPI_GetUser_NotFound(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	req := httptest.NewRequest("GET", "/api/admin/users/nonexistent", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAdminAPI_CreateUser(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	body := bytes.NewBufferString(`{"username":"carol","password":"password123","email":"carol@example.com"}`)
	req := httptest.NewRequest("POST", "/api/admin/users", body)
	req.Header.Set("X-API-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAPI_SetUserQuota(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	_, err := repo.CreateUser(t.Context(), "dave", "password123", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	body := `{"quota_bytes":10737418240}`
	req := httptest.NewRequest("PUT", "/api/admin/users/dave/quota", bytes.NewReader([]byte(body)))
	req.Header.Set("X-API-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify quota was set.
	user, _ := repo.GetUserByUsername(t.Context(), "dave")
	if user.StorageQuota != 10737418240 {
		t.Fatalf("expected quota 10737418240, got %d", user.StorageQuota)
	}
}

func TestAdminAPI_DisableUser(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	_, err := repo.CreateUser(t.Context(), "eve", "password123", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	body := `{"disabled":true}`
	req := httptest.NewRequest("PUT", "/api/admin/users/eve/disable", bytes.NewReader([]byte(body)))
	req.Header.Set("X-API-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	user, _ := repo.GetUserByUsername(t.Context(), "eve")
	if !user.Disabled {
		t.Fatal("expected user to be disabled")
	}
}

func TestAdminAPI_DeleteUser(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	_, err := repo.CreateUser(t.Context(), "frank", "password123", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/api/admin/users/frank", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	user, _ := repo.GetUserByUsername(t.Context(), "frank")
	if user != nil {
		t.Fatal("expected user to be deleted")
	}
}

func TestAdminAPI_Stats(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	req := httptest.NewRequest("GET", "/api/admin/stats", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp statsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestAdminAPI_OpenAPISpec(t *testing.T) {
	spec := AdminOpenAPISpec()
	if spec.Info.Title != "rstash Admin API" {
		t.Fatalf("unexpected title: %s", spec.Info.Title)
	}
	if spec.Paths.Len() == 0 {
		t.Fatal("expected paths in spec")
	}
}
