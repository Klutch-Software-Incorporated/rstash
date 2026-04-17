package web

import (
	"rstash/internal/auth"
	"rstash/internal/config"
	"rstash/internal/db"
	"rstash/internal/email"
	"rstash/internal/settings"
	"rstash/internal/storage"
	"rstash/internal/ui"
	"rstash/internal/webhooks"
)

// UIDeps holds the dependencies needed by UI handlers.
type UIDeps struct {
	Auth          auth.Service
	Repo          *db.Repository
	Renderer      *ui.Renderer
	Config        *config.Config
	Storage       *storage.Service
	Settings      *settings.Settings
	Mailer        email.Mailer        // nil when email not configured
	Webhooks      *webhooks.Emitter   // nil disables webhook emission
	SecureCookies bool   // true when base URL scheme is https
	LogFile       string // path to log file (empty = no file logging)
}
