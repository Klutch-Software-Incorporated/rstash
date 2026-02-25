package api_test

import (
	"testing"

	"gosilo/internal/metrics"
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
