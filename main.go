package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
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
)

func main() {
	cmd := "serve"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "serve":
		runServe()
	case "env":
		fmt.Print(config.GenerateEnvFile())
	case "version":
		fmt.Printf("gosilo %s\n", config.Version)
	case "help", "-h", "--help":
		printHelp(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "gosilo: unknown command %q\n\n", cmd)
		printHelp(os.Stderr)
		os.Exit(1)
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintf(w, "gosilo %s — remoteStorage server\n\n", config.Version)
	fmt.Fprintln(w, "Usage: gosilo [command]")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  serve    Start the server (default)")
	fmt.Fprintln(w, "  env      Print a documented .env template")
	fmt.Fprintln(w, "  version  Print version")
	fmt.Fprintln(w, "  help     Show this help")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Environment variables:")
	fmt.Fprintln(w, "")

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  Variable\tDefault\tDescription")
	fmt.Fprintln(tw, "  --------\t-------\t-----------")
	for _, v := range config.EnvVars() {
		def := v.Default
		if def == "" {
			def = "-"
		}
		desc := v.Description
		if len(v.ValidValues) > 0 {
			desc += " [" + strings.Join(v.ValidValues, ", ") + "]"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\n", v.Name, def, desc)
	}
	tw.Flush()
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Run 'gosilo env' to generate a documented .env template.")
}

func runServe() {
	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "configuration error:\n%v\n", err)
		os.Exit(1)
	}

	// Configure structured logging with dynamic level.
	var levelVar slog.LevelVar
	levelVar.Set(parseLogLevel(cfg.LogLevel))
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: &levelVar})))

	// Open and migrate the database.
	database, err := db.Open(cfg.DatabasePath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Initialize runtime settings (DB overrides + env defaults).
	runtimeSettings := settings.New(database, cfg)

	// Register onChange callback for dynamic log level.
	runtimeSettings.OnChange(func(s *settings.Snapshot) {
		levelVar.Set(parseLogLevel(s.LogLevel))
	})

	// Initialize blob storage.
	var blobs blob.Store
	switch cfg.BlobBackend {
	case "fs":
		if cfg.BlobPath == "" {
			slog.Error("GOSILO_BLOB_PATH is required when GOSILO_BLOB_BACKEND=fs")
			os.Exit(1)
		}
		blobs, err = blob.NewFSStore(cfg.BlobPath)
	case "sqlite":
		blobs, err = blob.NewSQLiteStore(database)
	default:
		slog.Error("unknown blob backend", "backend", cfg.BlobBackend)
		os.Exit(1)
	}
	if err != nil {
		slog.Error("failed to initialize blob store", "error", err)
		os.Exit(1)
	}

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

	mux.Handle("/.well-known/webfinger", api.CORS(api.WebFinger(cfg)))

	// OAuth routes (need auth loader + setup guard for session cookie support).
	oauthH := web.OAuthHandler(uiDeps)
	oauthWrap := func(h http.HandlerFunc) http.Handler {
		return web.AuthLoader(localAuth)(
			web.SetupGuard(localAuth)(http.HandlerFunc(h)),
		)
	}
	mux.Handle("GET /oauth/authorize", oauthWrap(oauthH.ShowAuthorize))
	mux.Handle("POST /oauth/authorize", oauthWrap(web.RequireCSRF(oauthH.DoAuthorize)))
	mux.Handle("POST /oauth/token", api.CORS(api.OAuthToken(database)))
	mux.Handle("/storage/{user}/{path...}", api.CORS(api.Storage(database, storageSvc, func() int64 {
		return runtimeSettings.Load().MaxUploadSize
	})))

	// Static file server from embedded assets.
	staticFS, err := fs.Sub(ui.Static, "static")
	if err != nil {
		slog.Error("failed to create static sub-filesystem", "error", err)
		os.Exit(1)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Web UI routes (must be registered last so specific routes take precedence).
	uiHandler := web.Routes(uiDeps)

	// Wrap UI handler with auth loader and setup guard.
	wrapped := web.AuthLoader(localAuth)(
		web.SetupGuard(localAuth)(uiHandler),
	)
	mux.Handle("/", wrapped)

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

func parseLogLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
