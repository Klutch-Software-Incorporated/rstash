package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"rstash/internal/api"
	"rstash/internal/auth"
	"rstash/internal/db"
	"rstash/internal/metrics"
)

func TestRouteGroup(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/storage/user/path/to/file", "storage"},
		{"/storage/user/", "storage"},
		{"/.well-known/webfinger", "webfinger"},
		{"/oauth/token", "oauth"},
		{"/oauth/authorize", "oauth"},
		{"/oauth/revoke", "oauth"},
		{"/metrics", "metrics"},
		{"/", "web"},
		{"/admin/users", "web"},
		{"/settings", "web"},
		{"/login", "web"},
		{"/static/style.css", "web"},
	}

	for _, tt := range tests {
		got := metrics.RouteGroup(tt.path)
		if got != tt.want {
			t.Errorf("RouteGroup(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestMetricsAuth_NoSession(t *testing.T) {
	repo, err := db.OpenRepository("sqlite::memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer repo.Close()

	authSvc := auth.NewLocalService(repo)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := api.MetricsAuth(authSvc, false, inner)
	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without session, got %d", rr.Code)
	}
}

func TestMetricsAuth_AdminSession(t *testing.T) {
	repo, err := db.OpenRepository("sqlite::memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer repo.Close()

	authSvc := auth.NewLocalService(repo)

	// Create admin user and session.
	user, err := repo.CreateUser(context.Background(), "admin", "password123", true, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sess, err := repo.CreateSession(context.Background(), user.ID, "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	var called bool
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := api.MetricsAuth(authSvc, false, inner)
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sess.Token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 for admin session, got %d", rr.Code)
	}
	if !called {
		t.Fatal("inner handler was not called for admin session")
	}
}

func TestMetricsAuth_NonAdminSession(t *testing.T) {
	repo, err := db.OpenRepository("sqlite::memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer repo.Close()

	authSvc := auth.NewLocalService(repo)

	// Create non-admin user and session.
	user, err := repo.CreateUser(context.Background(), "regular", "password123", false, true)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sess, err := repo.CreateSession(context.Background(), user.ID, "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := api.MetricsAuth(authSvc, false, inner)
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sess.Token})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin session, got %d", rr.Code)
	}
}
