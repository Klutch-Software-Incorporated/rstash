package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// RequireAPIKey returns middleware that validates the admin API key.
// Checks X-API-Key header first, then falls back to Authorization: Bearer.
// Returns 503 if no API key is configured, 401 if the key is missing or wrong.
func RequireAPIKey(apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiKey == "" {
				http.Error(w, `{"error":"admin API not configured"}`, http.StatusServiceUnavailable)
				return
			}

			provided := r.Header.Get("X-API-Key")
			if provided == "" {
				if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
					provided = strings.TrimPrefix(auth, "Bearer ")
				}
			}

			if provided == "" {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"API key required"}`, http.StatusUnauthorized)
				return
			}

			if subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
