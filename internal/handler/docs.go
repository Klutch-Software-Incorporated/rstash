package handler

import (
	"net/http"

	"gosilo/api"
)

const redocHTML = `<!DOCTYPE html>
<html>
<head>
    <title>Gosilo API Documentation</title>
    <meta charset="utf-8"/>
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>body { margin: 0; padding: 0; }</style>
</head>
<body>
    <redoc spec-url="/docs/openapi.yaml"></redoc>
    <script src="https://cdn.redoc.ly/redoc/latest/bundles/redoc.standalone.js"></script>
</body>
</html>`

// Docs returns a handler that serves the Redoc documentation page and
// the OpenAPI spec file.
func Docs() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /docs/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Write(api.OpenAPISpec)
	})

	mux.HandleFunc("GET /docs/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(redocHTML))
	})

	return mux
}
