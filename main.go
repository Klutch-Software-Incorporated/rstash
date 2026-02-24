package main

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gosilo/internal/blob"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/handler"
	"gosilo/internal/storage"
	"gosilo/internal/ui"
)

func main() {
	cfg := config.Load()

	// Configure structured logging.
	var level slog.Level
	switch cfg.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	// Open and migrate the database.
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Initialize blob storage.
	blobs, err := blob.NewSQLiteStore(database)
	if err != nil {
		slog.Error("failed to initialize blob store", "error", err)
		os.Exit(1)
	}

	// Initialize storage service.
	storageSvc := storage.NewService(database, blobs)

	// Initialize template renderer.
	renderer := ui.NewRenderer()

	// UI dependencies (shared by UI handlers and OAuth).
	uiDeps := &handler.UIDeps{
		DB:       database,
		Renderer: renderer,
		Config:   cfg,
	}

	// Build routes.
	mux := http.NewServeMux()

	mux.Handle("GET /.well-known/webfinger", handler.WebFinger(cfg))

	// OAuth routes (need auth loader + setup guard for session cookie support).
	oauthH := handler.OAuthHandler(uiDeps)
	oauthWrap := func(h http.HandlerFunc) http.Handler {
		return handler.AuthLoader(database)(
			handler.SetupGuard(database)(http.HandlerFunc(h)),
		)
	}
	mux.Handle("GET /oauth/authorize", oauthWrap(oauthH.ShowAuthorize))
	mux.Handle("POST /oauth/authorize", oauthWrap(oauthH.DoAuthorize))
	mux.Handle("POST /oauth/token", handler.OAuthToken())
	mux.Handle("/storage/{user}/{path...}", handler.Storage(database, storageSvc))

	// API docs (Redoc + OpenAPI spec).
	mux.Handle("/docs/", handler.Docs())

	// Static file server from embedded assets.
	staticFS, err := fs.Sub(ui.Static, "static")
	if err != nil {
		slog.Error("failed to create static sub-filesystem", "error", err)
		os.Exit(1)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Web UI routes (must be registered last so specific routes take precedence).
	uiHandler := handler.UI(uiDeps)

	// Wrap UI handler with auth loader and setup guard.
	wrapped := handler.AuthLoader(database)(
		handler.SetupGuard(database)(uiHandler),
	)
	mux.Handle("/", wrapped)

	// Start server.
	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: handler.RequestLogger(mux),
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Session cleanup goroutine.
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := db.DeleteExpiredSessions(context.Background(), database); err != nil {
					slog.Error("failed to delete expired sessions", "error", err)
				}
				if err := db.DeleteExpiredOAuthTokens(context.Background(), database); err != nil {
					slog.Error("failed to delete expired oauth tokens", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		slog.Info("server starting", "addr", cfg.Addr, "base_url", cfg.BaseURL)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down server")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}
