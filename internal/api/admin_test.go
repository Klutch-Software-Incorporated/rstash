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

func TestAdminAPI_CreateUser_Provision(t *testing.T) {
	deps, repo := setupAdminTest(t)
	deps.BaseURL = "https://rstash.example.com"
	handler := AdminRoutes(deps)

	body := `{"username":"provisioned","email":"p@example.com","email_verified":true,"provision":true,"quota_bytes":1073741824}`
	req := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader([]byte(body)))
	req.Header.Set("X-API-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data                  apiUser `json:"data"`
		ClaimURL              string  `json:"claim_url"`
		ClaimTokenExpiresAt   string  `json:"claim_token_expires_at"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.ClaimURL == "" {
		t.Fatal("expected claim_url in response")
	}
	if resp.ClaimTokenExpiresAt == "" {
		t.Fatal("expected claim_token_expires_at in response")
	}

	user, err := repo.GetUserByUsername(t.Context(), "provisioned")
	if err != nil {
		t.Fatal(err)
	}
	if user == nil {
		t.Fatal("user not created")
	}
	if !user.ExternallyManaged {
		t.Error("expected ExternallyManaged=true")
	}
	if !user.EmailVerified {
		t.Error("expected EmailVerified=true")
	}
	if user.StorageQuota != 1073741824 {
		t.Errorf("expected quota 1073741824, got %d", user.StorageQuota)
	}
	if user.PasswordResetToken == nil || *user.PasswordResetToken == "" {
		t.Error("expected claim token to be set")
	}
	if !user.IsUnclaimed() {
		t.Error("expected user to be IsUnclaimed")
	}
}

func TestAdminAPI_CreateUser_ProvisionRequiresNoPassword(t *testing.T) {
	deps, _ := setupAdminTest(t)
	handler := AdminRoutes(deps)

	// Missing both password AND provision flag should 400.
	body := `{"username":"foo","email":"foo@example.com"}`
	req := httptest.NewRequest("POST", "/api/admin/users", bytes.NewReader([]byte(body)))
	req.Header.Set("X-API-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
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

func TestAdminAPI_SetUserEmail(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	_, err := repo.CreateUser(t.Context(), "emailed", "password123", "old@example.com", false, true)
	if err != nil {
		t.Fatal(err)
	}

	body := `{"email":"new@example.com","verified":true}`
	req := httptest.NewRequest("PUT", "/api/admin/users/emailed/email", bytes.NewReader([]byte(body)))
	req.Header.Set("X-API-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	user, _ := repo.GetUserByUsername(t.Context(), "emailed")
	if user.Email == nil || *user.Email != "new@example.com" {
		t.Fatalf("expected email to be updated, got %v", user.Email)
	}
	if !user.EmailVerified {
		t.Error("expected EmailVerified=true")
	}
}

func TestAdminAPI_SetUserEmail_Conflict(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	_, _ = repo.CreateUser(t.Context(), "alice", "password123", "alice@example.com", false, true)
	_, _ = repo.CreateUser(t.Context(), "bob", "password123", "bob@example.com", false, true)

	body := `{"email":"alice@example.com"}`
	req := httptest.NewRequest("PUT", "/api/admin/users/bob/email", bytes.NewReader([]byte(body)))
	req.Header.Set("X-API-Key", testAPIKey)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAPI_DisableUser(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	_, err := repo.CreateUser(t.Context(), "eve", "password123", "", false, true)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/admin/users/eve/disable", nil)
	req.Header.Set("X-API-Key", testAPIKey)
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

func TestAdminAPI_EnableUser(t *testing.T) {
	deps, repo := setupAdminTest(t)
	handler := AdminRoutes(deps)

	u, err := repo.CreateUser(t.Context(), "gina", "password123", "", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateUserDisabled(t.Context(), u.ID, true); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/api/admin/users/gina/enable", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	user, _ := repo.GetUserByUsername(t.Context(), "gina")
	if user.Disabled {
		t.Fatal("expected user to be enabled (Disabled=false)")
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
