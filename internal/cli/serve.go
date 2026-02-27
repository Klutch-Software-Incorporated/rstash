package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"gosilo/internal/api"
	"gosilo/internal/auth"
	"gosilo/internal/blob"
	"gosilo/internal/cmdinfo"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/metrics"
	"gosilo/internal/settings"
	"gosilo/internal/storage"
	"gosilo/internal/ui"
	"gosilo/internal/web"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"
)

var webModeFlag string

var serveCmd = &cobra.Command{
	Use:     "serve",
	Short:   "Start the server",
	Long:    "Start the HTTP server with storage API, OAuth, and optional web UI.",
	GroupID: "server",
	Example: `  gosilo serve
  gosilo serve --web=oauth
  GOSILO_ADDR=:9090 gosilo serve`,
	RunE: runServe,
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
	var logWriter io.Writer = os.Stderr
	if cfg.LogFile != "" {
		logFile, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file %q: %w", cfg.LogFile, err)
		}
		defer logFile.Close()
		logWriter = io.MultiWriter(os.Stderr, logFile)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logWriter, &slog.HandlerOptions{Level: &levelVar})))

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

	// Initialize content scanner.
	mimeScanner := storage.NewMIMEScanner(func() string {
		return runtimeSettings.Load().BlockedMIMETypes
	})
	storageSvc.SetScanner(mimeScanner)

	// Initialize auth service.
	localAuth := auth.NewLocalService(database)

	// Initialize template renderer.
	renderer := ui.NewRenderer()

	// Compute effective TLS mode for serving.
	effectiveTLSMode := cfg.TLSMode
	if effectiveTLSMode == "" {
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			effectiveTLSMode = "manual"
		} else {
			effectiveTLSMode = "off"
		}
	}

	secureCookies := strings.HasPrefix(cfg.BaseURL, "https://") ||
		effectiveTLSMode == "manual" || effectiveTLSMode == "auto"

	// UI dependencies (shared by UI handlers and OAuth).
	uiDeps := &web.UIDeps{
		Auth:          localAuth,
		DB:            database,
		Renderer:      renderer,
		Config:        cfg,
		Storage:       storageSvc,
		Settings:      runtimeSettings,
		SecureCookies: secureCookies,
		LogFile:       cfg.LogFile,
		CommandIndex:  cmdinfo.WalkCommands(rootCmd),
	}

	// Build routes.
	mux := http.NewServeMux()

	// Always registered: WebFinger, OAuth token, storage API, metrics.
	metricsH := promhttp.Handler()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		switch runtimeSettings.Load().MetricsMode {
		case "off":
			http.NotFound(w, r)
		case "admin":
			api.MetricsAuth(database, localAuth, secureCookies, metricsH).ServeHTTP(w, r)
		default: // "public"
			metricsH.ServeHTTP(w, r)
		}
	})
	mux.Handle("/.well-known/webfinger", api.CORS(api.WebFinger(cfg)))
	mux.Handle("POST /oauth/token", api.CORS(api.OAuthToken(database,
		func() string { return runtimeSettings.Load().TokenLifetime },
		func() (bool, string) {
			snap := runtimeSettings.Load()
			return snap.RefreshTokens == "enabled", snap.RefreshTokenLifetime
		},
	)))
	mux.Handle("POST /oauth/revoke", api.CORS(api.OAuthRevoke(database)))
	mux.Handle("/storage/{user}/{path...}", api.CORS(api.Storage(database, storageSvc, func() int64 {
		return runtimeSettings.Load().MaxUploadSize
	})))

	// JSON management API (registered outside web_mode gating).
	jsonH := web.JSONApiHandler(uiDeps)
	jsonH.RegisterRoutes(mux)

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
			return web.AuthLoader(localAuth, secureCookies)(
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
		wrapped := web.AuthLoader(localAuth, secureCookies)(
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
	var handler http.Handler = api.MetricsMiddleware(api.RateLimit(limiter)(mux))
	if snap.RateLimitRate > 0 {
		slog.Info("rate limiting enabled", "rate", snap.RateLimitRate, "burst", snap.RateLimitBurst)
	}

	// Start server.
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.RequestLogger(api.SecurityHeaders(api.SecurityHeadersConfig{
			HTTPS: strings.HasPrefix(cfg.BaseURL, "https://"),
		})(handler)),
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
				if err := db.DeleteExpiredRefreshTokens(context.Background(), database); err != nil {
					slog.Error("failed to delete expired refresh tokens", "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Metrics gauge updater goroutine.
	go func() {
		// Derive the disk path for available space checks.
		var diskPath string
		switch blobScheme {
		case "fs":
			diskPath = blobPath
		case "sqlite":
			diskPath = filepath.Dir(blobPath)
		}

		update := func() {
			bgCtx := context.Background()
			if total, err := db.GetTotalStorageUsed(bgCtx, database); err == nil {
				metrics.StorageUsedBytes.Set(float64(total))
			}
			if diskPath != "" {
				if avail, err := metrics.DiskAvailableBytes(diskPath); err == nil {
					metrics.StorageAvailableBytes.Set(float64(avail))
				}
			}
			if count, err := db.UserCount(bgCtx, database); err == nil {
				metrics.UsersTotal.Set(float64(count))
			}
			if count, err := db.CountActiveSessions(bgCtx, database); err == nil {
				metrics.ActiveSessions.Set(float64(count))
			}
			if count, err := db.CountActiveOAuthTokens(bgCtx, database); err == nil {
				metrics.ActiveTokens.Set(float64(count))
			}
		}

		update() // initial population
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				update()
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		switch effectiveTLSMode {
		case "auto":
			hostname := extractHostname(cfg.BaseURL)
			m := &autocert.Manager{
				Prompt:     autocert.AcceptTOS,
				HostPolicy: autocert.HostWhitelist(hostname),
				Cache:      autocert.DirCache(cfg.TLSCacheDir),
			}
			srv.TLSConfig = m.TLSConfig()

			// HTTP server for ACME challenges + HTTP→HTTPS redirect.
			httpSrv := &http.Server{
				Addr:    ":80",
				Handler: m.HTTPHandler(nil), // nil = redirect to HTTPS
			}
			go httpSrv.ListenAndServe()

			slog.Info("server starting (autocert)", "addr", cfg.Addr,
				"base_url", cfg.BaseURL, "hostname", hostname)
			if err := srv.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}

		case "manual":
			slog.Info("server starting (TLS)", "addr", cfg.Addr, "base_url", cfg.BaseURL, "web_mode", cfg.WebMode)
			if err := srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}

		default: // "off"
			if !isLocalhost(cfg.Addr) && !strings.HasPrefix(cfg.BaseURL, "https://") {
				slog.Warn("running without TLS on a non-localhost address",
					"addr", cfg.Addr,
					"hint", "set GOSILO_TLS_MODE=auto for Let's Encrypt, or use a reverse proxy")
			}
			slog.Info("server starting", "addr", cfg.Addr, "base_url", cfg.BaseURL, "web_mode", cfg.WebMode)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}
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

// isLocalhost reports whether the listen address host is localhost or empty.
func isLocalhost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	return host == "" || host == "127.0.0.1" || host == "::1" || host == "localhost"
}

// extractHostname parses a URL and returns just the hostname (no port).
func extractHostname(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
