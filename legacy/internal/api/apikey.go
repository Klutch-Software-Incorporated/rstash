package api

import (
	"context"
	"math"
	"net/http"
	"strconv"
	"strings"

	"rstash/internal/db"
	"rstash/internal/model"
)

type apiKeyContextKey struct{}

// CurrentAPIKey returns the APIKey that authenticated the request,
// or nil if none (e.g. in unit tests).
func CurrentAPIKey(r *http.Request) *model.APIKey {
	k, _ := r.Context().Value(apiKeyContextKey{}).(*model.APIKey)
	return k
}

// RequireAPIKey returns middleware that validates the admin API key by looking
// it up in the database (see APIKey model). Keys are created via the admin UI.
//
// Checks X-API-Key header first, then falls back to Authorization: Bearer.
// Returns 401 if the key is missing, unknown, or doesn't match any stored hash.
// On success, the matching *APIKey is placed in the request context and is
// accessible via CurrentAPIKey(r). Enforces the key's per-key RateLimitRPM.
//
// If limiter is nil, rate limiting is skipped (useful in tests).
func RequireAPIKey(repo *db.Repository, limiter *APIKeyRateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			key, err := repo.FindAPIKeyByRaw(r.Context(), provided)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"failed to validate key"}`, http.StatusInternalServerError)
				return
			}
			if key == nil {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"invalid API key"}`, http.StatusUnauthorized)
				return
			}

			if limiter != nil {
				allowed, retryAfter := limiter.Allow(key.ID, key.RateLimitRPM)
				if !allowed {
					secs := int(math.Ceil(retryAfter.Seconds()))
					if secs < 1 {
						secs = 1
					}
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("Retry-After", strconv.Itoa(secs))
					http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
					return
				}
			}

			ctx := context.WithValue(r.Context(), apiKeyContextKey{}, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
