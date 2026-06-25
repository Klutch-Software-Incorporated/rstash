package api

import (
	"net/http"
	"strconv"
	"time"

	"rstash/internal/auth"
	"rstash/internal/metrics"
)

// MetricsMiddleware records HTTP request counts and durations.
func MetricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		route := metrics.RouteGroup(r.URL.Path)
		status := strconv.Itoa(rw.status)
		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, status, route).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// MetricsAuth guards the /metrics endpoint, requiring an admin session cookie
// or an admin bearer token.
func MetricsAuth(authSvc auth.Service, secureCookies bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try session cookie first.
		if cookie, err := r.Cookie(auth.SessionCookieName); err == nil && cookie.Value != "" {
			sess, err := authSvc.GetSession(r.Context(), cookie.Value)
			if err == nil && sess != nil {
				user, err := authSvc.GetUser(r.Context(), sess.UserID)
				if err == nil && user != nil && user.IsAdmin {
					next.ServeHTTP(w, r)
					return
				}
			}
		}

		http.Error(w, "Forbidden", http.StatusForbidden)
	})
}
