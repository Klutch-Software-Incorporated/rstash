package handler

import (
	"net/http"

	"gosilo/internal/blob"
)

// Storage handles GET/PUT/DELETE/HEAD requests on /storage/{user}/{path...}.
func Storage(_ blob.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, DELETE, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Origin, If-Match, If-None-Match")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Type, Content-Length, ETag")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		http.Error(w, "storage: not yet implemented", http.StatusNotImplemented)
	})
}
