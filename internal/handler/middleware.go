package handler

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"gosilo/internal/db"
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
func AuthLoader(database *sql.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("gosilo_session")
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			sess, err := db.GetSessionByToken(r.Context(), database, cookie.Value)
			if err != nil {
				slog.Error("failed to get session", "error", err)
				next.ServeHTTP(w, r)
				return
			}
			if sess == nil {
				// Expired or invalid — clear cookie.
				http.SetCookie(w, &http.Cookie{
					Name:     "gosilo_session",
					Value:    "",
					Path:     "/",
					MaxAge:   -1,
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				next.ServeHTTP(w, r)
				return
			}

			user, err := db.GetUserByID(r.Context(), database, sess.UserID)
			if err != nil || user == nil {
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
func SetupGuard(database *sql.DB) func(http.Handler) http.Handler {
	var hasUsers atomic.Bool

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/setup" || path == "/login" || path == "/register" {
				next.ServeHTTP(w, r)
				return
			}

			if !hasUsers.Load() {
				count, err := db.UserCount(r.Context(), database)
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

// SetFlash sets a flash message cookie.
func SetFlash(w http.ResponseWriter, message string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "gosilo_flash",
		Value:    message,
		Path:     "/",
		MaxAge:   60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// GetFlash reads and clears the flash message cookie.
func GetFlash(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("gosilo_flash")
	if err != nil {
		return ""
	}
	// Clear it.
	http.SetCookie(w, &http.Cookie{
		Name:     "gosilo_flash",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return cookie.Value
}

// SetSessionCookie sets the session cookie.
func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "gosilo_session",
		Value:    token,
		Path:     "/",
		MaxAge:   7 * 24 * int(time.Hour/time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie removes the session cookie.
func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "gosilo_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// CORS wraps a handler with the CORS headers required by
// draft-dejong-remotestorage-26 (storage API and WebFinger).
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers",
			"Authorization, Content-Length, Content-Type, Origin, X-Requested-With, If-Match, If-None-Match")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Type, ETag")
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
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
