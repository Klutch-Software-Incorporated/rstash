package api

import "net/http"

// OAuthToken handles POST /oauth/token (stub for future PKCE support).
func OAuthToken() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "oauth token: not yet implemented", http.StatusNotImplemented)
	})
}
