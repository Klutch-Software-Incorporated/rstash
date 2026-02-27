package web_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"gosilo/internal/auth"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/settings"
	"gosilo/internal/ui"
	"gosilo/internal/web"
)

// jsonApiSetup creates a test server with the JSON API mux, enables json_api=admin,
// creates an admin user, logs in, and returns the server URL, deps, and Bearer token.
func jsonApiSetup(t *testing.T) (*httptest.Server, *web.UIDeps, string) {
	t.Helper()

	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := &config.Config{
		Addr:             ":8080",
		BaseURL:          "http://localhost:8080",
		DatabaseDSN:      "sqlite::memory:",
		BlobDSN:          "sqlite::memory:",
		RegistrationMode: "closed",
		LogLevel:         "error",
		JSONApi:          "off",
	}

	localAuth := auth.NewLocalService(database)
	runtimeSettings := settings.New(database, cfg)
	renderer := ui.NewRenderer()

	deps := &web.UIDeps{
		Auth:     localAuth,
		DB:       database,
		Renderer: renderer,
		Config:   cfg,
		Settings: runtimeSettings,
	}

	// Enable JSON API via runtime settings.
	if err := runtimeSettings.Set(context.Background(), "json_api", "admin"); err != nil {
		t.Fatalf("enable json_api: %v", err)
	}

	// Create the JSON API mux.
	mux := http.NewServeMux()
	jsonH := web.JSONApiHandler(deps)
	jsonH.RegisterRoutes(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	// Create an admin user.
	_, err = db.CreateUser(context.Background(), database, "admin", "adminpass123", true, true)
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	// Login to get a Bearer token.
	token := jsonLogin(t, ts, "admin", "adminpass123")

	return ts, deps, token
}

// jsonLogin logs in via the JSON API and returns the auth token.
func jsonLogin(t *testing.T, ts *httptest.Server, username, password string) string {
	t.Helper()
	resp, result := jsonPost(t, ts, "/json/login", "", map[string]string{
		"username": username,
		"password": password,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", resp.StatusCode)
	}
	payload, ok := result["payload"].(map[string]any)
	if !ok {
		t.Fatal("login: payload not a map")
	}
	token, ok := payload["authToken"].(string)
	if !ok || token == "" {
		t.Fatal("login: missing authToken in payload")
	}
	return token
}

// jsonPost sends a POST request with JSON body and optional Bearer token.
func jsonPost(t *testing.T, ts *httptest.Server, path, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", ts.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(data, &result)
	return resp, result
}

// jsonGet sends a GET request with optional Bearer token.
func jsonGet(t *testing.T, ts *httptest.Server, path, token string) (*http.Response, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest("GET", ts.URL+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var result map[string]any
	json.Unmarshal(data, &result)
	return resp, result
}

// --- Tests ---

func TestJSONApi_DisabledReturns404(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		Addr:        ":8080",
		BaseURL:     "http://localhost:8080",
		DatabaseDSN: "sqlite::memory:",
		BlobDSN:     "sqlite::memory:",
		LogLevel:    "error",
		JSONApi:     "off",
	}
	localAuth := auth.NewLocalService(database)
	runtimeSettings := settings.New(database, cfg)

	deps := &web.UIDeps{
		Auth:     localAuth,
		DB:       database,
		Renderer: ui.NewRenderer(),
		Config:   cfg,
		Settings: runtimeSettings,
	}

	mux := http.NewServeMux()
	web.JSONApiHandler(deps).RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/json/user/list")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when json_api=off, got %d", resp.StatusCode)
	}
}

func TestJSONApi_LoginReturns404WhenDisabled(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		Addr:        ":8080",
		BaseURL:     "http://localhost:8080",
		DatabaseDSN: "sqlite::memory:",
		BlobDSN:     "sqlite::memory:",
		LogLevel:    "error",
		JSONApi:     "off",
	}
	localAuth := auth.NewLocalService(database)
	runtimeSettings := settings.New(database, cfg)

	deps := &web.UIDeps{
		Auth:     localAuth,
		DB:       database,
		Renderer: ui.NewRenderer(),
		Config:   cfg,
		Settings: runtimeSettings,
	}

	mux := http.NewServeMux()
	web.JSONApiHandler(deps).RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	body := bytes.NewReader([]byte(`{"username":"x","password":"y"}`))
	resp, err := http.Post(ts.URL+"/json/login", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for login when json_api=off, got %d", resp.StatusCode)
	}
}

func TestJSONApi_LoginSuccess(t *testing.T) {
	ts, _, _ := jsonApiSetup(t)

	resp, result := jsonPost(t, ts, "/json/login", "", map[string]string{
		"username": "admin",
		"password": "adminpass123",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatal("expected ok:true")
	}
	payload := result["payload"].(map[string]any)
	if _, ok := payload["authToken"].(string); !ok {
		t.Fatal("expected authToken in payload")
	}
}

func TestJSONApi_LoginInvalidCredentials(t *testing.T) {
	ts, _, _ := jsonApiSetup(t)

	resp, result := jsonPost(t, ts, "/json/login", "", map[string]string{
		"username": "admin",
		"password": "wrongpassword",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	if result["ok"] != false {
		t.Fatal("expected ok:false")
	}
}

func TestJSONApi_LoginNonAdminForbidden(t *testing.T) {
	ts, deps, _ := jsonApiSetup(t)

	// Create a non-admin user.
	_, err := db.CreateUser(context.Background(), deps.DB, "regular", "password123", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	resp, result := jsonPost(t, ts, "/json/login", "", map[string]string{
		"username": "regular",
		"password": "password123",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
	if result["ok"] != false {
		t.Fatal("expected ok:false")
	}
}

func TestJSONApi_BearerTokenAuth(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	resp, result := jsonGet(t, ts, "/json/user/list", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 with Bearer token, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatal("expected ok:true")
	}
}

func TestJSONApi_NoBearerReturns401(t *testing.T) {
	ts, _, _ := jsonApiSetup(t)

	resp, result := jsonGet(t, ts, "/json/user/list", "")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without auth, got %d", resp.StatusCode)
	}
	if result["ok"] != false {
		t.Fatal("expected ok:false")
	}
}

func TestJSONApi_ContentTypeRequired(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	// POST without Content-Type: application/json.
	req, _ := http.NewRequest("POST", ts.URL+"/json/user/add", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+token)
	// Deliberately omit Content-Type.
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("expected 415 without proper Content-Type, got %d", resp.StatusCode)
	}
}

func TestJSONApi_UserList(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	resp, result := jsonGet(t, ts, "/json/user/list", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	payload := result["payload"].(map[string]any)
	users, ok := payload["users"].([]any)
	if !ok {
		t.Fatal("expected users array in payload")
	}
	if len(users) < 1 {
		t.Fatal("expected at least 1 user")
	}
}

func TestJSONApi_UserAddAndDelete(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	// Add a user.
	resp, result := jsonPost(t, ts, "/json/user/add", token, map[string]any{
		"username": "newuser",
		"password": "newpass1234",
		"is_admin": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add user: expected 200, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatalf("add user: expected ok:true, got %v", result)
	}

	// Verify the user appears in list.
	_, listResult := jsonGet(t, ts, "/json/user/list", token)
	payload := listResult["payload"].(map[string]any)
	users := payload["users"].([]any)
	found := false
	for _, u := range users {
		um := u.(map[string]any)
		if um["username"] == "newuser" {
			found = true
		}
	}
	if !found {
		t.Fatal("newly created user not in list")
	}

	// Delete the user.
	resp, result = jsonPost(t, ts, "/json/user/delete", token, map[string]string{
		"username": "newuser",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete user: expected 200, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatal("delete user: expected ok:true")
	}

	// Verify user is gone.
	_, listResult = jsonGet(t, ts, "/json/user/list", token)
	payload = listResult["payload"].(map[string]any)
	users = payload["users"].([]any)
	for _, u := range users {
		um := u.(map[string]any)
		if um["username"] == "newuser" {
			t.Fatal("deleted user still in list")
		}
	}
}

func TestJSONApi_UserPasswd(t *testing.T) {
	ts, deps, token := jsonApiSetup(t)

	// Create a target user.
	_, err := db.CreateUser(context.Background(), deps.DB, "pwduser", "oldpass1234", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Change password.
	resp, result := jsonPost(t, ts, "/json/user/passwd", token, map[string]string{
		"username": "pwduser",
		"password": "newpass5678",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("passwd: expected 200, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatal("passwd: expected ok:true")
	}

	// Verify old password no longer works: promote user to admin first so login doesn't 403.
	if err := deps.Auth.ToggleAdmin(context.Background(), 2, true); err != nil {
		t.Fatalf("toggle admin: %v", err)
	}
	resp, _ = jsonPost(t, ts, "/json/login", "", map[string]string{
		"username": "pwduser",
		"password": "oldpass1234",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password should fail, got %d", resp.StatusCode)
	}

	// Verify new password works.
	resp, _ = jsonPost(t, ts, "/json/login", "", map[string]string{
		"username": "pwduser",
		"password": "newpass5678",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new password should work, got %d", resp.StatusCode)
	}
}

func TestJSONApi_UserPromote(t *testing.T) {
	ts, deps, token := jsonApiSetup(t)

	_, err := db.CreateUser(context.Background(), deps.DB, "promuser", "password123", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Promote.
	resp, result := jsonPost(t, ts, "/json/user/promote", token, map[string]string{
		"username": "promuser",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("promote: expected 200, got %d", resp.StatusCode)
	}
	payload := result["payload"].(map[string]any)
	if payload["is_admin"] != true {
		t.Fatal("expected is_admin:true after promote")
	}

	// Demote.
	resp, result = jsonPost(t, ts, "/json/user/promote", token, map[string]string{
		"username": "promuser",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("demote: expected 200, got %d", resp.StatusCode)
	}
	payload = result["payload"].(map[string]any)
	if payload["is_admin"] != false {
		t.Fatal("expected is_admin:false after demote")
	}
}

func TestJSONApi_UserDisable(t *testing.T) {
	ts, deps, token := jsonApiSetup(t)

	_, err := db.CreateUser(context.Background(), deps.DB, "disuser", "password123", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	// Disable.
	resp, result := jsonPost(t, ts, "/json/user/disable", token, map[string]string{
		"username": "disuser",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable: expected 200, got %d", resp.StatusCode)
	}
	payload := result["payload"].(map[string]any)
	if payload["disabled"] != true {
		t.Fatal("expected disabled:true")
	}

	// Re-enable.
	resp, result = jsonPost(t, ts, "/json/user/disable", token, map[string]string{
		"username": "disuser",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-enable: expected 200, got %d", resp.StatusCode)
	}
	payload = result["payload"].(map[string]any)
	if payload["disabled"] != false {
		t.Fatal("expected disabled:false after re-enable")
	}
}

func TestJSONApi_ConfigList(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	resp, result := jsonGet(t, ts, "/json/config/list", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	payload := result["payload"].(map[string]any)
	settings, ok := payload["settings"].([]any)
	if !ok {
		t.Fatal("expected settings array in payload")
	}
	if len(settings) == 0 {
		t.Fatal("expected at least one setting")
	}

	// Check that known keys exist.
	keys := make(map[string]bool)
	for _, s := range settings {
		sm := s.(map[string]any)
		keys[sm["key"].(string)] = true
	}
	for _, expected := range []string{"metrics_mode", "json_api", "log_level"} {
		if !keys[expected] {
			t.Fatalf("expected setting key %q in config list", expected)
		}
	}
}

func TestJSONApi_ConfigGetSet(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	// Set log_level to debug.
	resp, result := jsonPost(t, ts, "/json/config/set", token, map[string]string{
		"key":   "log_level",
		"value": "debug",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config set: expected 200, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatal("config set: expected ok:true")
	}

	// Get and verify.
	resp, result = jsonGet(t, ts, "/json/config/get?key=log_level", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config get: expected 200, got %d", resp.StatusCode)
	}
	payload := result["payload"].(map[string]any)
	if payload["value"] != "debug" {
		t.Fatalf("expected value=debug, got %v", payload["value"])
	}
	if payload["source"] != "db" {
		t.Fatalf("expected source=db after set, got %v", payload["source"])
	}
}

func TestJSONApi_ConfigReset(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	// Set then reset.
	jsonPost(t, ts, "/json/config/set", token, map[string]string{
		"key":   "log_level",
		"value": "debug",
	})

	resp, result := jsonPost(t, ts, "/json/config/reset", token, map[string]string{
		"key": "log_level",
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config reset: expected 200, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatal("config reset: expected ok:true")
	}

	// Verify source is no longer "db".
	_, getResult := jsonGet(t, ts, "/json/config/get?key=log_level", token)
	payload := getResult["payload"].(map[string]any)
	if payload["source"] == "db" {
		t.Fatal("expected source != db after reset")
	}
}

func TestJSONApi_AuditTail(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	// The login in setup generates audit entries. Fetch them.
	resp, result := jsonGet(t, ts, "/json/audit/tail", token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatal("expected ok:true")
	}
	payload := result["payload"].(map[string]any)
	entries, ok := payload["entries"].([]any)
	if !ok {
		t.Fatal("expected entries array in payload")
	}
	// We at least have the login audit entry from setup.
	if len(entries) < 1 {
		t.Fatal("expected at least 1 audit entry")
	}
}

func TestJSONApi_OpenAPISpec(t *testing.T) {
	ts, _, _ := jsonApiSetup(t)

	resp, err := http.Get(ts.URL + "/json/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "text/yaml; charset=utf-8" {
		t.Fatalf("expected text/yaml content type, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("openapi:")) {
		t.Fatal("response body does not contain 'openapi:'")
	}
}

func TestJSONApi_DocsPage(t *testing.T) {
	ts, _, _ := jsonApiSetup(t)

	resp, err := http.Get(ts.URL + "/json/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !bytes.Contains(body, []byte("redoc")) {
		t.Fatal("response body does not contain 'redoc'")
	}
}

func TestJSONApi_DocsDisabled(t *testing.T) {
	database, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	cfg := &config.Config{
		Addr:        ":8080",
		BaseURL:     "http://localhost:8080",
		DatabaseDSN: "sqlite::memory:",
		BlobDSN:     "sqlite::memory:",
		LogLevel:    "error",
		JSONApi:     "off",
	}
	localAuth := auth.NewLocalService(database)
	runtimeSettings := settings.New(database, cfg)

	deps := &web.UIDeps{
		Auth:     localAuth,
		DB:       database,
		Renderer: ui.NewRenderer(),
		Config:   cfg,
		Settings: runtimeSettings,
	}

	mux := http.NewServeMux()
	web.JSONApiHandler(deps).RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Docs page should 404 when json_api=off.
	resp, err := http.Get(ts.URL + "/json/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for docs when json_api=off, got %d", resp.StatusCode)
	}

	// OpenAPI spec should also 404.
	resp, err = http.Get(ts.URL + "/json/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for openapi.yaml when json_api=off, got %d", resp.StatusCode)
	}
}

func TestJSONApi_Logout(t *testing.T) {
	ts, _, token := jsonApiSetup(t)

	// Logout.
	resp, result := jsonPost(t, ts, "/json/logout", token, map[string]string{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: expected 200, got %d", resp.StatusCode)
	}
	if result["ok"] != true {
		t.Fatal("logout: expected ok:true")
	}

	// Token should now be invalid.
	resp, result = jsonGet(t, ts, "/json/user/list", token)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", resp.StatusCode)
	}
}
