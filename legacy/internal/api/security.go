package api

import (
	"net/http"
	"strings"
)

// SecurityHeadersConfig controls which security headers are emitted.
type SecurityHeadersConfig struct {
	HTTPS bool // true when base URL scheme is https
}

// SecurityHeaders wraps a handler to set standard security response headers.
// CSP allows the Tailwind CDN. HSTS is added only when serving over HTTPS.
func SecurityHeaders(cfg SecurityHeadersConfig) func(http.Handler) http.Handler {
	csp := strings.Join([]string{
		"default-src 'self'",
		"script-src 'self' https://cdn.tailwindcss.com 'unsafe-inline'",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"frame-ancestors 'none'",
	}, "; ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Content-Security-Policy", csp)
			if cfg.HTTPS {
				w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
