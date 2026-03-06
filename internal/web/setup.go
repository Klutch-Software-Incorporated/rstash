package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gosilo/internal/auth"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/ui"
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
	Error    string
}

type setupReviewContent struct {
	Settings []setupSetting
	Warnings []string
}

type setupSetting struct {
	Label string
	Value string
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
			Title:     "Setup — Gosilo",
			CSRFToken: CSRFToken(r),
			Content:   &setupContent{},
		})
		return
	}

	h.deps.Renderer.Render(w, "setup_review", ui.PageData{
		Title:   "Setup — Gosilo",
		Content: h.buildReview(),
	})
}

func (h *setupHandler) buildReview() *setupReviewContent {
	cfg := h.deps.Config

	dbType := friendlyDSNType(cfg.DatabaseDSN)
	blobType := friendlyDSNType(cfg.BlobDSN)

	settings := []setupSetting{
		{Label: "Server address", Value: cfg.BaseURL},
		{Label: "Database", Value: dbType},
		{Label: "File storage", Value: blobType},
		{Label: "TLS", Value: friendlyTLS(cfg)},
	}

	var warnings []string
	if strings.HasPrefix(cfg.DatabaseDSN, "sqlite:") {
		warnings = append(warnings, "The database is set to SQLite (the default). This works well for personal and small-group use. If you plan to use PostgreSQL, MySQL, or SQL Server instead, stop the server now and set the GOSILO_DB environment variable before continuing.")
	}
	if strings.HasPrefix(cfg.BlobDSN, "sqlite:") {
		warnings = append(warnings, "File storage is set to SQLite (the default). For larger deployments, consider using filesystem storage (fs:) or S3 by setting the GOSILO_BLOB environment variable.")
	}

	return &setupReviewContent{
		Settings: settings,
		Warnings: warnings,
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

	username, valErr := db.ValidateUsername(r.FormValue("username"))

	renderErr := func(msg string) {
		h.deps.Renderer.Render(w, "setup", ui.PageData{
			Title:     "Setup — Gosilo",
			CSRFToken: CSRFToken(r),
			Content:   &setupContent{Username: username, Error: msg},
		})
	}

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

	user, err := h.deps.Auth.CreateUser(r.Context(), username, password, true, true)
	if err != nil {
		slog.Error("failed to create admin user", "error", err)
		renderErr("Failed to create user. Username may already be taken.")
		return
	}

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
	ui.SetFlash(w, "Welcome to Gosilo! Your admin account has been created.")
	http.Redirect(w, r, "/admin", http.StatusSeeOther)
}
