package web

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"

	"gosilo/internal/auth"
	"gosilo/internal/model"
	"gosilo/internal/ui"
)

type contextKey string

const (
	ctxKeyUser    contextKey = "user"
	ctxKeySession contextKey = "session"
)

// AuthLoader returns middleware that reads the session cookie and loads
// the user into the request context.
func AuthLoader(authSvc auth.Service, secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("gosilo_session")
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

			if user.Disabled {
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

// CSRFToken returns the CSRF token for the current session, or empty string.
func CSRFToken(r *http.Request) string {
	if s := CurrentSession(r); s != nil {
		return s.CSRFToken
	}
	return ""
}

// ValidateCSRF checks that the form's csrf_token matches the session's CSRF token.
// Returns true if valid.
func ValidateCSRF(r *http.Request) bool {
	sess := CurrentSession(r)
	if sess == nil {
		return false
	}
	return r.FormValue("csrf_token") == sess.CSRFToken
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
