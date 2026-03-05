package api

import (
	"net/http"

	"gosilo/internal/db"
)

// Health returns a handler that checks database connectivity and returns
// 200 OK or 503 Service Unavailable. Intended for load balancer probes.
func Health(repo *db.Repository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := repo.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("database unreachable"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}
}
