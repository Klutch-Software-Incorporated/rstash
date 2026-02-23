package handler

import (
	"net/http"
)

// OAuthAuthorize handles GET /oauth/authorize.
func OAuthAuthorize() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "oauth authorize: not yet implemented", http.StatusNotImplemented)
	})
}

// OAuthToken handles POST /oauth/token.
func OAuthToken() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "oauth token: not yet implemented", http.StatusNotImplemented)
	})
}
