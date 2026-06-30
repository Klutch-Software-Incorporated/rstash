package auth

import (
	"net/http"
	"time"
)

// SessionCookieName is the name of the session cookie.
const SessionCookieName = "rstash_session"

// SetSessionCookie sets the session cookie. When secure is true, the Secure
// flag is set (should be true when serving over HTTPS). When domain is
// non-empty, the cookie is scoped to that domain (enables subdomain sharing);
// empty means host-only.
func SetSessionCookie(w http.ResponseWriter, token, domain string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    token,
		Path:     "/",
		Domain:   domain,
		MaxAge:   7 * 24 * int(time.Hour/time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}

// ClearSessionCookie removes the session cookie. The domain must match the
// domain the cookie was set with, or the browser will not clear it.
func ClearSessionCookie(w http.ResponseWriter, domain string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		Domain:   domain,
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	})
}
