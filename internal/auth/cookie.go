package auth

import (
	"net/http"
	"time"
)

// SetSessionCookie sets the session cookie. When secure is true, the Secure
// flag is set (should be true when serving over HTTPS).
func SetSessionCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "gosilo_session",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * int(time.Hour/time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "gosilo_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}
