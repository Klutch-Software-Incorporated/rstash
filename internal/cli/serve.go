package cli

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gosilo/internal/api"
	"gosilo/internal/auth"
	"gosilo/internal/blob"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/settings"
	"gosilo/internal/storage"
	"gosilo/internal/ui"
	"gosilo/internal/web"

	"github.com/spf13/cobra"
)

var webModeFlag string

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the server",
	RunE:  runServe,
}

func init() {
	serveCmd.Flags().StringVar(&webModeFlag, "web", "", "web UI mode: full, oauth, off (overrides GOSILO_WEB_MODE)")
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg := config.Load()

	// Apply --db flag override.
	if dbFlag != "" {
		cfg.DatabaseDSN = dbFlag
	}

	// Apply --web flag override.
	if webModeFlag != "" {
		cfg.WebMode = webModeFlag
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration error:\n%w", err)
	}

	// Configure structured logging with dynamic level.
	var levelVar slog.LevelVar
	levelVar.Set(parseLogLevel(cfg.LogLevel))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: &levelVar})))

	// Open and migrate the metadata database.
	database, err := db.Open(cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer database.Close()

	// Initialize runtime settings (DB overrides + env defaults).
	runtimeSettings := settings.New(database, cfg)

	// Register onChange callback for dynamic log level.
	runtimeSettings.OnChange(func(s *settings.Snapshot) {
		levelVar.Set(parseLogLevel(s.LogLevel))
	})

	// Initialize blob storage from DSN.
	blobScheme, blobPath, _ := config.ParseDSN(cfg.BlobDSN) // already validated
	var blobs blob.Store
	switch blobScheme {
	case "fs":
		blobs, err = blob.NewFSStore(blobPath)
	case "sqlite":
		blobs, err = blob.NewSQLiteStore(blobPath)
	}
	if err != nil {
		return fmt.Errorf("failed to initialize blob store: %w", err)
	}
	defer blobs.Close()

	// Initialize quota checker (always create so it can be enabled at runtime).
	snap := runtimeSettings.Load()
	quotaChecker := storage.NewQuotaChecker(storage.QuotaConfig{
		Mode:       snap.QuotaMode,
		TotalLimit: snap.QuotaTotal,
		UserLimit:  snap.QuotaUser,
	}, database)

	runtimeSettings.OnChange(func(s *settings.Snapshot) {
		quotaChecker.UpdateConfig(storage.QuotaConfig{
			Mode:       s.QuotaMode,
			TotalLimit: s.QuotaTotal,
			UserLimit:  s.QuotaUser,
		})
	})

	// Initialize storage service.
	storageSvc := storage.NewService(database, blobs, quotaChecker)

	// Initialize auth service.
	localAuth := auth.NewLocalService(database)

	// Initialize template renderer.
	renderer := ui.NewRenderer()

	// UI dependencies (shared by UI handlers and OAuth).
	uiDeps := &web.UIDeps{
		Auth:     localAuth,
		DB:       database,
		Renderer: renderer,
		Config:   cfg,
		Storage:  storageSvc,
		Settings: runtimeSettings,
	}

	// Build routes.
	mux := http.NewServeMux()

	// Always registered: WebFinger, OAuth token, storage API.
	mux.Handle("/.well-known/webfinger", api.CORS(api.WebFinger(cfg)))
	mux.Handle("POST /oauth/token", api.CORS(api.OAuthToken(database)))
	mux.Handle("/storage/{user}/{path...}", api.CORS(api.Storage(database, storageSvc, func() int64 {
		return runtimeSettings.Load().MaxUploadSize
	})))

	// Web mode gating.
	if cfg.WebMode != "off" {
		// Static file server from embedded assets.
		staticFS, err := fs.Sub(ui.Static, "static")
		if err != nil {
			return fmt.Errorf("failed to create static sub-filesystem: %w", err)
		}
		mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

		// OAuth authorize routes (need auth loader + setup guard for session cookie support).
		oauthH := web.OAuthHandler(uiDeps)
		oauthWrap := func(h http.HandlerFunc) http.Handler {
			return web.AuthLoader(localAuth)(
				web.SetupGuard(localAuth)(http.HandlerFunc(h)),
			)
		}
		mux.Handle("GET /oauth/authorize", oauthWrap(oauthH.ShowAuthorize))
		mux.Handle("POST /oauth/authorize", oauthWrap(web.RequireCSRF(oauthH.DoAuthorize)))

		// Choose routes based on web mode.
		var uiHandler http.Handler
		if cfg.WebMode == "full" {
			uiHandler = web.FullRoutes(uiDeps)
		} else {
			uiHandler = web.OAuthRoutes(uiDeps)
		}

		// Wrap UI handler with auth loader and setup guard.
		wrapped := web.AuthLoader(localAuth)(
			web.SetupGuard(localAuth)(uiHandler),
		)
		mux.Handle("/", wrapped)
	}

	// Always create rate limiter so it can be enabled/adjusted at runtime.
	limiter := api.NewRateLimiter(snap.RateLimitRate, snap.RateLimitBurst)
	defer limiter.Stop()
	runtimeSettings.OnChange(func(s *settings.Snapshot) {
		limiter.UpdateConfig(s.RateLimitRate, s.RateLimitBurst)
	})
	var handler http.Handler = api.RateLimit(limiter)(mux)
	if snap.RateLimitRate > 0 {
		slog.Info("rate limiting enabled", "rate", snap.RateLimitRate, "burst", snap.RateLimitBurst)
	}

	// Start server.
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.RequestLogger(api.SecurityHeaders(handler)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
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
				if err := localAuth.CleanupExpiredSessions(context.Background()); err != nil {
					slog.Error("failed to delete expired sessions", "error", err)
				}
				if err := db.DeleteExpiredOAuthTokens(context.Background(), database); err != nil {
					slog.Error("failed to delete expired oauth tokens", "error", err)
				}
				if err := db.DeleteExpiredAuthorizationCodes(context.Background(), database); err != nil {
					slog.Error("failed to delete expired authorization codes", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		slog.Info("server starting", "addr", cfg.Addr, "base_url", cfg.BaseURL, "web_mode", cfg.WebMode)
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

	return nil
}
