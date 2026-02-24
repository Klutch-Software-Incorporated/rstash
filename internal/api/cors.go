package api

import "net/http"

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
