package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"rstash/internal/auth"
	"rstash/internal/config"
	"rstash/internal/db"
	"rstash/internal/metrics"
	"rstash/internal/ui"
)

type setupHandler struct {
	deps *UIDeps
}

// SetupHandler returns handler methods for the setup wizard.
func SetupHandler(deps *UIDeps) *setupHandler {
	return &setupHandler{deps: deps}
}

type setupContent struct {
	Username string
	Email    string
	Error    string
}

type setupReviewContent struct {
	Settings []setupSetting
}

type setupSetting struct {
	Label   string
	Status  string // short status text
	Level   string // "ok", "info", "warn", "muted"
	Detail  string // expandable help text, empty if none
	Tooltip string // hover tooltip, empty if none
}

func (h *setupHandler) ShowSetup(w http.ResponseWriter, r *http.Request) {
	// If users already exist, redirect to home.
	count, err := h.deps.Auth.UserCount(r.Context())
	if err != nil {
		slog.Error("failed to check user count", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if r.URL.Query().Get("step") == "account" {
		h.deps.Renderer.Render(w, "setup", ui.PageData{
			Title:     "Setup — rstash",
			CSRFToken: CSRFToken(r),
			Content:   &setupContent{},
		})
		return
	}

	h.deps.Renderer.Render(w, "setup_review", ui.PageData{
		Title:   "Setup — rstash",
		Content: h.buildReview(),
	})
}

func (h *setupHandler) buildReview() *setupReviewContent {
	cfg := h.deps.Config

	// Server URL
	urlLevel := "ok"
	urlStatus := cfg.BaseURL
	if strings.Contains(cfg.BaseURL, "localhost") || strings.Contains(cfg.BaseURL, "127.0.0.1") {
		urlLevel = "muted"
		urlStatus = "Localhost"
	}

	// Database
	dbLevel := "ok"
	dbStatus := friendlyDSNType(cfg.DatabaseDSN)
	if strings.HasPrefix(cfg.DatabaseDSN, "sqlite:") {
		dbLevel = "info"
		dbStatus = "Default (SQLite)"
	}

	// Blob storage
	blobLevel := "ok"
	blobStatus := friendlyDSNType(cfg.BlobDSN)
	if strings.HasPrefix(cfg.BlobDSN, "sqlite:") {
		blobLevel = "info"
		blobStatus = "Default (SQLite)"
	}

	// TLS
	tlsLevel := "ok"
	tlsStatus := friendlyTLS(cfg)
	if tlsStatus == "Off" {
		tlsLevel = "warn"
	}

	// Email
	emailLevel := "ok"
	emailStatus := "Configured"
	if cfg.EmailDSN == "" {
		emailLevel = "muted"
		emailStatus = "Not set"
	}

	// Listen address
	addrStatus := cfg.Addr
	if addrStatus == "" {
		addrStatus = ":8080"
	}

	return &setupReviewContent{
		Settings: []setupSetting{
			{Label: "Server URL", Status: urlStatus, Level: urlLevel,
				Detail: "The public URL used for WebFinger discovery and OAuth redirects. Set RSTASH_BASE_URL to your production domain."},
			{Label: "Listen address", Status: addrStatus, Level: "ok"},
			{Label: "Database", Status: dbStatus, Level: dbLevel,
				Detail:  "Stores users, sessions, and file metadata. SQLite works well for personal use. For larger deployments, set RSTASH_DB to a PostgreSQL, MySQL, or SQL Server DSN.",
				Tooltip: "RSTASH_DB"},
			{Label: "File storage", Status: blobStatus, Level: blobLevel,
				Detail:  "Stores uploaded file contents. SQLite is simple but can grow large. Set RSTASH_BLOB to use filesystem (fs:) or S3-compatible (s3:) storage instead.",
				Tooltip: "RSTASH_BLOB"},
			{Label: "TLS", Status: tlsStatus, Level: tlsLevel,
				Detail:  "Encrypts traffic between clients and the server. Set RSTASH_TLS_MODE to auto for Let\u2019s Encrypt, manual for your own certificate, or use a reverse proxy like Caddy or nginx.",
				Tooltip: "RSTASH_TLS_MODE"},
			{Label: "Email", Status: emailStatus, Level: emailLevel,
				Detail:  "Required for password reset and email verification. Set RSTASH_EMAIL to an email provider DSN (e.g. resend:API_KEY?from=noreply@example.com).",
				Tooltip: "RSTASH_EMAIL"},
		},
	}
}

// friendlyDSNType returns a user-friendly label for a DSN scheme.
func friendlyDSNType(dsn string) string {
	scheme, _, err := config.ParseDSN(dsn)
	if err != nil {
		return "Unknown"
	}
	switch scheme {
	case "sqlite":
		return "SQLite (default)"
	case "postgres":
		return "PostgreSQL"
	case "mysql":
		return "MySQL"
	case "mssql":
		return "SQL Server"
	case "fs":
		return "Filesystem"
	case "s3":
		return "S3-compatible storage"
	default:
		return scheme
	}
}

// friendlyTLS returns a user-friendly TLS description.
func friendlyTLS(cfg *config.Config) string {
	switch cfg.TLSMode {
	case "auto":
		return "Automatic (Let's Encrypt)"
	case "manual":
		return "Enabled (manual certificate)"
	case "off":
		return "Off"
	default:
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			return "Enabled (manual certificate)"
		}
		if strings.HasPrefix(cfg.BaseURL, "https://") {
			return "Expected (base URL uses https)"
		}
		return "Off"
	}
}

func (h *setupHandler) DoSetup(w http.ResponseWriter, r *http.Request) {
	// If users already exist, redirect to home.
	count, err := h.deps.Auth.UserCount(r.Context())
	if err != nil {
		slog.Error("failed to check user count", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	if count > 0 {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("password_confirm")
	emailRaw := r.FormValue("email")

	username, valErr := db.ValidateUsername(r.FormValue("username"))

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "setup", ui.PageData{
			Title:     "Setup — rstash",
			CSRFToken: CSRFToken(r),
			Content:   &setupContent{Username: username, Email: emailRaw, Error: msg},
		})
	}

	if valErr != nil {
		renderErr(valErr.Error())
		return
	}

	email, valErr := db.ValidateEmail(emailRaw)
	if valErr != nil {
		renderErr(valErr.Error())
		return
	}

	if msg := validatePassword(password); msg != "" {
		renderErr(msg)
		return
	}
	if password != confirm {
		renderErr("Passwords do not match.")
		return
	}

	user, err := h.deps.Auth.CreateUser(r.Context(), username, password, email, true, true)
	if err != nil {
		slog.Error("failed to create admin user", "error", err)
		renderErr("Failed to create user. Username may already be taken.")
		return
	}

	metrics.UserSignupsTotal.Inc()

	// Auto-accept TOS/Privacy for the initial admin (operator-created).
	_ = h.deps.Repo.AcceptTOS(r.Context(), user.ID)
	_ = h.deps.Repo.AcceptPrivacy(r.Context(), user.ID)

	// Create session.
	sess, err := h.deps.Auth.CreateSession(r.Context(), user.ID, ClientIP(r))
	if err != nil {
		slog.Error("failed to create session", "error", err)
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.deps.Repo.Audit(r.Context(), user.ID, "setup.completed", "user", fmt.Sprintf("%d", user.ID), username)
	auth.SetSessionCookie(w, sess.Token, h.deps.SecureCookies)
	ui.SetFlash(w, "Welcome to rstash! Your admin account has been created.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
