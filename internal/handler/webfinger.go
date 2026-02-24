package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"gosilo/internal/config"
)

// WebFinger handles GET /.well-known/webfinger requests for remoteStorage discovery.
func WebFinger(cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		resource := r.URL.Query().Get("resource")
		if resource == "" {
			http.Error(w, "missing resource parameter", http.StatusBadRequest)
			return
		}

		// Parse acct:user@host
		if !strings.HasPrefix(resource, "acct:") {
			http.Error(w, "invalid resource format", http.StatusBadRequest)
			return
		}

		parts := strings.SplitN(strings.TrimPrefix(resource, "acct:"), "@", 2)
		if len(parts) != 2 {
			http.Error(w, "invalid resource format", http.StatusBadRequest)
			return
		}
		username := parts[0]

		storageHref := cfg.BaseURL + "/storage/" + username
		authURL := cfg.BaseURL + "/oauth/authorize"

		resp := map[string]any{
			"subject": resource,
			"links": []map[string]any{
				{
					"href": storageHref,
					"rel":  "http://tools.ietf.org/id/draft-dejong-remotestorage",
					"type": "draft-dejong-remotestorage-26",
					"properties": map[string]string{
						"http://remotestorage.io/spec/version": "draft-dejong-remotestorage-26",
						"http://tools.ietf.org/html/rfc6749#section-4.2": authURL,
					},
				},
			},
		}

		w.Header().Set("Content-Type", "application/jrd+json")
		json.NewEncoder(w).Encode(resp)
	})
}
