package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPIKeyRateLimiter_Disabled(t *testing.T) {
	rl := NewAPIKeyRateLimiter()
	defer rl.Stop()

	for i := 0; i < 100; i++ {
		allowed, _ := rl.Allow(1, 0)
		if !allowed {
			t.Fatalf("request %d denied when rate=0", i)
		}
	}
}

func TestAPIKeyRateLimiter_Enforces(t *testing.T) {
	rl := NewAPIKeyRateLimiter()
	defer rl.Stop()

	// Rate limit of 60 rpm = burst 60. First 60 should succeed, 61st should fail.
	for i := 0; i < 60; i++ {
		allowed, _ := rl.Allow(1, 60)
		if !allowed {
			t.Fatalf("request %d unexpectedly denied within burst", i)
		}
	}
	allowed, retry := rl.Allow(1, 60)
	if allowed {
		t.Fatal("expected 61st request to be denied")
	}
	if retry <= 0 {
		t.Errorf("expected positive retry-after, got %v", retry)
	}
}

func TestAPIKeyRateLimiter_PerKey(t *testing.T) {
	rl := NewAPIKeyRateLimiter()
	defer rl.Stop()

	// Exhaust key 1's burst, key 2 should remain unaffected.
	for i := 0; i < 60; i++ {
		rl.Allow(1, 60)
	}
	if allowed, _ := rl.Allow(1, 60); allowed {
		t.Fatal("key 1 should be rate-limited")
	}
	if allowed, _ := rl.Allow(2, 60); !allowed {
		t.Fatal("key 2 should be unaffected by key 1")
	}
}

func TestAdminAPI_RateLimitExceeded(t *testing.T) {
	deps, _ := setupAdminTest(t)
	deps.RateLimiter = NewAPIKeyRateLimiter()
	defer deps.RateLimiter.Stop()

	// Shrink the test key's limit to 1 RPM so we hit the cap quickly.
	key, _ := deps.Repo.FindAPIKeyByRaw(t.Context(), testAPIKey)
	key.RateLimitRPM = 1
	if err := deps.Repo.UpdateAPIKey(t.Context(), key); err != nil {
		t.Fatal(err)
	}

	handler := AdminRoutes(deps)

	// First request: allowed.
	req := httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", w.Code)
	}

	// Second request: should exceed the burst (1).
	req = httptest.NewRequest("GET", "/api/admin/users", nil)
	req.Header.Set("X-API-Key", testAPIKey)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429")
	}
}
