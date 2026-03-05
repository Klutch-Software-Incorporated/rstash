package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"sync/atomic"

	"gosilo/internal/auth"
	"gosilo/internal/model"
	"gosilo/internal/ui"
)

type contextKey string

const (
	ctxKeyUser       contextKey = "user"
	ctxKeySession    contextKey = "session"
	ctxKeyTargetUser contextKey = "targetUser"
	ctxKeyIsSelf     contextKey = "isSelf"
	ctxKeyCSRFCookie contextKey = "csrfCookie"
)

const csrfCookieName = "gosilo_csrf"

// AuthLoader returns middleware that reads the session cookie and loads
// the user into the request context.
func AuthLoader(authSvc auth.Service, secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(auth.SessionCookieName)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			sess, err := authSvc.GetSession(r.Context(), cookie.Value)
			if err != nil {
				slog.Error("failed to get session", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if sess == nil {
				// Expired or invalid — clear cookie.
				auth.ClearSessionCookie(w, secureCookies)
				next.ServeHTTP(w, r)
				return
			}

			user, err := authSvc.GetUser(r.Context(), sess.UserID)
			if err != nil || user == nil {
				next.ServeHTTP(w, r)
				return
			}

			if user.Disabled || !user.Approved {
				auth.ClearSessionCookie(w, secureCookies)
				next.ServeHTTP(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyUser, user)
			ctx = context.WithValue(ctx, ctxKeySession, sess)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CurrentUser returns the authenticated user from context, or nil.
func CurrentUser(r *http.Request) *model.User {
	u, _ := r.Context().Value(ctxKeyUser).(*model.User)
	return u
}

// CurrentSession returns the session from context, or nil.
func CurrentSession(r *http.Request) *model.Session {
	s, _ := r.Context().Value(ctxKeySession).(*model.Session)
	return s
}

// EnsureCSRFCookie is middleware that sets a gosilo_csrf cookie on every response
// if one is not already present. The cookie value is stored in context so that
// CSRFToken() can return it for template rendering on pre-auth pages.
func EnsureCSRFCookie(secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var token string
			if c, err := r.Cookie(csrfCookieName); err == nil && c.Value != "" {
				token = c.Value
			} else {
				b := make([]byte, 16)
				if _, err := rand.Read(b); err != nil {
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				token = hex.EncodeToString(b)
				http.SetCookie(w, &http.Cookie{
					Name:     csrfCookieName,
					Value:    token,
					Path:     "/",
					HttpOnly: true,
					Secure:   secureCookies,
					SameSite: http.SameSiteLaxMode,
				})
			}
			ctx := context.WithValue(r.Context(), ctxKeyCSRFCookie, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// csrfCookieToken returns the CSRF cookie value from context, or empty string.
func csrfCookieToken(r *http.Request) string {
	v, _ := r.Context().Value(ctxKeyCSRFCookie).(string)
	return v
}

// CSRFToken returns the CSRF token for the current request. It prefers the
// session-based token when a session exists, and falls back to the double-submit
// cookie token for pre-auth pages (login, setup, register, abuse report).
func CSRFToken(r *http.Request) string {
	if s := CurrentSession(r); s != nil {
		return s.CSRFToken
	}
	return csrfCookieToken(r)
}

// ValidateCSRF checks that the form's csrf_token matches either the session's
// CSRF token or the double-submit cookie token. Returns true if valid.
func ValidateCSRF(r *http.Request) bool {
	formToken := r.FormValue("csrf_token")
	if formToken == "" {
		return false
	}
	// Prefer session-based CSRF when a session exists.
	if sess := CurrentSession(r); sess != nil {
		return formToken == sess.CSRFToken
	}
	// Fall back to double-submit cookie comparison.
	cookieToken := csrfCookieToken(r)
	return cookieToken != "" && formToken == cookieToken
}

// RequireCSRF wraps a handler to reject requests with an invalid CSRF token.
func RequireCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ValidateCSRF(r) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// AdminGuard wraps an http.HandlerFunc to require authentication and admin status.
// Use this to wrap all admin route handlers instead of manual checks in each handler.
func AdminGuard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !user.IsAdmin {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// RequireAuth redirects to /login if not authenticated.
func RequireAuth(w http.ResponseWriter, r *http.Request) bool {
	if CurrentUser(r) == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return false
	}
	return true
}

// RequireAdmin returns 403 if the user is not an admin.
func RequireAdmin(w http.ResponseWriter, r *http.Request) bool {
	user := CurrentUser(r)
	if user == nil || !user.IsAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// SetupGuard returns middleware that redirects to /setup when no users exist.
func SetupGuard(authSvc auth.Service) func(http.Handler) http.Handler {
	var hasUsers atomic.Bool

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/setup" || path == "/login" || path == "/register" {
				next.ServeHTTP(w, r)
				return
			}

			if !hasUsers.Load() {
				count, err := authSvc.UserCount(r.Context())
				if err != nil {
					slog.Error("failed to check user count", "error", err)
					next.ServeHTTP(w, r)
					return
				}
				if count == 0 {
					http.Redirect(w, r, "/setup", http.StatusSeeOther)
					return
				}
				hasUsers.Store(true)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// UserScopeGuard returns middleware that resolves {username} from the URL,
// verifies the current user is either the target user or an admin, and
// stores the target user + isSelf flag in context.
func UserScopeGuard(authSvc auth.Service) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			currentUser := CurrentUser(r)
			if currentUser == nil {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}

			username := r.PathValue("username")
			if username == "" {
				http.NotFound(w, r)
				return
			}

			targetUser, err := authSvc.GetUserByUsername(r.Context(), username)
			if err != nil || targetUser == nil {
				http.NotFound(w, r)
				return
			}

			isSelf := currentUser.ID == targetUser.ID
			if !isSelf && !currentUser.IsAdmin {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			ctx := context.WithValue(r.Context(), ctxKeyTargetUser, targetUser)
			ctx = context.WithValue(ctx, ctxKeyIsSelf, isSelf)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// TargetUser returns the target user from context (set by UserScopeGuard).
func TargetUser(r *http.Request) *model.User {
	u, _ := r.Context().Value(ctxKeyTargetUser).(*model.User)
	return u
}

// IsSelf returns true if the current user is viewing their own profile.
func IsSelf(r *http.Request) bool {
	v, _ := r.Context().Value(ctxKeyIsSelf).(bool)
	return v
}

// userInfo converts a model.User to a ui.UserInfo, or nil if user is nil.
func userInfo(u *model.User) *ui.UserInfo {
	if u == nil {
		return nil
	}
	return &ui.UserInfo{
		ID:       u.ID,
		Username: u.Username,
		IsAdmin:  u.IsAdmin,
	}
}
