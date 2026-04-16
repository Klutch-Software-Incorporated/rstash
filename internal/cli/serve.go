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

	"rstash/internal/api"
	"rstash/internal/auth"
	"rstash/internal/blob"
	"rstash/internal/config"
	"rstash/internal/db"
	"rstash/internal/email"
	"rstash/internal/metrics"
	"rstash/internal/settings"
	"rstash/internal/storage"
	"rstash/internal/ui"
	"rstash/internal/web"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/acme/autocert"
)

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

var serveCmd = &cobra.Command{
	Use:    "serve",
	Short:  "Start the server",
	Long:   "Start the HTTP server with storage API, OAuth, and web UI.",
	RunE:   runServe,
	Hidden: true, // Users just run "rstash" directly; serve is an alias.
}

func init() {
	rootCmd.AddCommand(serveCmd)
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg := config.Load()

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
	repo, err := db.OpenRepository(cfg.DatabaseDSN)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer repo.Close()

	// Initialize runtime settings (DB overrides + env defaults).
	runtimeSettings := settings.New(repo, cfg)

	// Register onChange callback for dynamic log level.
	runtimeSettings.OnChange(func(s *settings.Snapshot) {
		levelVar.Set(parseLogLevel(s.LogLevel))
	})

	// Auto-approve pending users when switching to "open" registration mode.
	prevRegMode := runtimeSettings.Load().RegistrationMode
	runtimeSettings.OnChange(func(s *settings.Snapshot) {
		if prevRegMode != "open" && s.RegistrationMode == "open" {
			n, err := repo.ApproveAllPending(context.Background())
			if err != nil {
				slog.Error("failed to auto-approve pending users", "error", err)
			} else if n > 0 {
				slog.Info("auto-approved pending users on mode switch to open", "count", n)
			}
		}
		prevRegMode = s.RegistrationMode
	})

	// Initialize blob storage from DSN.
	blobScheme, blobPath, _ := config.ParseDSN(cfg.BlobDSN) // already validated
	blobs, err := blob.OpenStore(blobScheme, blobPath, cfg.BlobDSN)
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
	}, repo)

	runtimeSettings.OnChange(func(s *settings.Snapshot) {
		quotaChecker.UpdateConfig(storage.QuotaConfig{
			Mode:       s.QuotaMode,
			TotalLimit: s.QuotaTotal,
			UserLimit:  s.QuotaUser,
		})
	})

	// Initialize storage service.
	storageSvc := storage.NewService(repo, blobs, quotaChecker)

	// Initialize content scanner.
	mimeScanner := storage.NewMIMEScanner(func() string {
		return runtimeSettings.Load().BlockedMIMETypes
	})
	storageSvc.SetScanner(mimeScanner)

	// Initialize email mailer (nil if not configured).
	mailer, err := email.Open(cfg.EmailDSN)
	if err != nil {
		return fmt.Errorf("failed to initialize email: %w", err)
	}
	if mailer != nil {
		slog.Info("email delivery enabled")
	}

	// Initialize auth service.
	localAuth := auth.NewLocalService(repo)

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
		Repo:          repo,
		Renderer:      renderer,
		Config:        cfg,
		Storage:       storageSvc,
		Settings:      runtimeSettings,
		Mailer:        mailer,
		SecureCookies: secureCookies,
		LogFile:       cfg.LogFile,
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
			api.MetricsAuth(localAuth, secureCookies, metricsH).ServeHTTP(w, r)
		default: // "public"
			metricsH.ServeHTTP(w, r)
		}
	})
	mux.Handle("GET /health", api.Health(repo))
	mux.Handle("/.well-known/webfinger", api.CORS(api.WebFinger(cfg)))
	mux.Handle("POST /oauth/token", api.CORS(api.OAuthToken(repo,
		func() string { return runtimeSettings.Load().TokenLifetime },
		func() (bool, string) {
			snap := runtimeSettings.Load()
			return snap.RefreshTokens == "enabled", snap.RefreshTokenLifetime
		},
	)))
	mux.Handle("POST /oauth/revoke", api.CORS(api.OAuthRevoke(repo)))
	mux.Handle("/storage/{user}/{path...}", api.CORS(api.Storage(repo, storageSvc, func() int64 {
		return runtimeSettings.Load().MaxUploadSize
	})))

	// Admin JSON API (keys managed via admin UI).
	apiKeyLimiter := api.NewAPIKeyRateLimiter()
	adminDeps := &api.AdminDeps{Repo: repo, Storage: storageSvc, BaseURL: cfg.BaseURL, RateLimiter: apiKeyLimiter}
	mux.Handle("/api/admin/", api.AdminRoutes(adminDeps))
	mux.HandleFunc("GET /api/admin/openapi.json", api.ServeOpenAPISpec(api.AdminOpenAPISpec()))

	// Static file server from embedded assets.
	staticFS, err := fs.Sub(ui.Static, "static")
	if err != nil {
		return fmt.Errorf("failed to create static sub-filesystem: %w", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Serve embedded site (Astro build output) for public pages.
	siteFS, _ := fs.Sub(ui.Site, "site")
	if _, err := fs.Stat(siteFS, "index.html"); err == nil {
		siteServer := http.FileServer(http.FS(siteFS))
		mux.Handle("GET /docs/", siteWithCache(siteServer))
		mux.Handle("GET /_astro/", siteWithCache(siteServer))
		mux.Handle("GET /screenshots/", siteServer)
		mux.Handle("GET /favicon.svg", siteServer)
	}

	// OAuth authorize routes (need auth loader + setup guard for session cookie support).
	oauthH := web.OAuthHandler(uiDeps)
	oauthWrap := func(h http.HandlerFunc) http.Handler {
		return web.AuthLoader(localAuth, runtimeSettings, secureCookies)(
			web.SetupGuard(localAuth)(http.HandlerFunc(h)),
		)
	}
	mux.Handle("GET /oauth/authorize", oauthWrap(oauthH.ShowAuthorize))
	mux.Handle("POST /oauth/authorize", oauthWrap(web.RequireCSRF(oauthH.DoAuthorize)))

	// Web UI routes.
	uiHandler := web.FullRoutes(uiDeps)
	wrapped := web.AuthLoader(localAuth, runtimeSettings, secureCookies)(
		web.EnsureCSRFCookie(runtimeSettings, secureCookies)(
			web.SetupGuard(localAuth)(
				web.AccountGuard()(uiHandler),
			),
		),
	)
	mux.Handle("/", wrapped)

	// Always create rate limiter so it can be enabled/adjusted at runtime.
	limiter := api.NewRateLimiter(snap.RateLimitRate, snap.RateLimitBurst)
	defer limiter.Stop()
	runtimeSettings.OnChange(func(s *settings.Snapshot) {
		limiter.UpdateConfig(s.RateLimitRate, s.RateLimitBurst)
	})
	var handler http.Handler = api.MetricsMiddleware(api.RateLimit(limiter)(api.MaxBodySize(1<<20)(mux)))
	if snap.RateLimitRate > 0 {
		slog.Info("rate limiting enabled", "rate", snap.RateLimitRate, "burst", snap.RateLimitBurst)
	}

	// Start server.
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.Recovery(api.RequestLogger(api.SecurityHeaders(api.SecurityHeadersConfig{
			HTTPS: strings.HasPrefix(cfg.BaseURL, "https://"),
		})(handler))),
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
				if err := repo.DeleteExpiredOAuthTokens(context.Background()); err != nil {
					slog.Error("failed to delete expired oauth tokens", "error", err)
				}
				if err := repo.DeleteExpiredAuthorizationCodes(context.Background()); err != nil {
					slog.Error("failed to delete expired authorization codes", "error", err)
				}
				if err := repo.DeleteExpiredRefreshTokens(context.Background()); err != nil {
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
			// Run all DB reads in a single transaction so we acquire the
			// connection once rather than competing for it 4 separate times.
			bgCtx := context.Background()
			_ = repo.Transaction(func(tx *db.Repository) error {
				if total, err := tx.GetTotalStorageUsed(bgCtx); err == nil {
					metrics.StorageUsedBytes.Set(float64(total))
				}
				if count, err := tx.UserCount(bgCtx); err == nil {
					metrics.UsersTotal.Set(float64(count))
				}
				if count, err := tx.CountActiveSessions(bgCtx); err == nil {
					metrics.ActiveSessions.Set(float64(count))
				}
				if count, err := tx.CountActiveOAuthTokens(bgCtx); err == nil {
					metrics.ActiveTokens.Set(float64(count))
				}
				return nil
			})
			// Disk space check is not a DB operation.
			if diskPath != "" {
				if avail, err := metrics.DiskAvailableBytes(diskPath); err == nil {
					metrics.StorageAvailableBytes.Set(float64(avail))
				}
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

			// HTTP server for ACME challenges + HTTP->HTTPS redirect.
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
			slog.Info("server starting (TLS)", "addr", cfg.Addr, "base_url", cfg.BaseURL)
			if err := srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey); err != nil && err != http.ErrServerClosed {
				slog.Error("server error", "error", err)
				os.Exit(1)
			}

		default: // "off"
			if !isLocalhost(cfg.Addr) && !strings.HasPrefix(cfg.BaseURL, "https://") {
				slog.Warn("running without TLS on a non-localhost address",
					"addr", cfg.Addr,
					"hint", "set RSTASH_TLS_MODE=auto for Let's Encrypt, or use a reverse proxy")
			}
			slog.Info("server starting", "addr", cfg.Addr, "base_url", cfg.BaseURL)
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

// siteWithCache wraps a handler to add immutable cache headers for content-hashed paths.
func siteWithCache(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/_astro/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		h.ServeHTTP(w, r)
	})
}

// extractHostname parses a URL and returns just the hostname (no port).
func extractHostname(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
