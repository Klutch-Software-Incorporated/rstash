package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery wraps a handler with panic recovery. If a handler panics, the
// middleware logs the panic value and stack trace, then responds with 500.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				stack := string(debug.Stack())
				slog.Error("panic recovered",
					"panic", v,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", stack,
				)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
