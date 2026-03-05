package api

import (
	"net/http"
	"strings"
)

// MaxBodySize wraps a handler to limit request body size on POST/PUT/PATCH
// requests. Storage API routes and multipart uploads are excluded because
// they apply their own per-request limits via MaxBytesReader.
func MaxBodySize(limit int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && (r.Method == "POST" || r.Method == "PUT" || r.Method == "PATCH") {
				if !strings.HasPrefix(r.URL.Path, "/storage/") &&
					!strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
					r.Body = http.MaxBytesReader(w, r.Body, limit)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
