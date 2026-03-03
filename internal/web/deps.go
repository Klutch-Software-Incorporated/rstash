package web

import (
	"gosilo/internal/auth"
	"gosilo/internal/cmdinfo"
	"gosilo/internal/config"
	"gosilo/internal/db"
	"gosilo/internal/settings"
	"gosilo/internal/storage"
	"gosilo/internal/ui"
)

// UIDeps holds the dependencies needed by UI handlers.
type UIDeps struct {
	Auth          auth.Service
	Repo          *db.Repository
	Renderer      *ui.Renderer
	Config        *config.Config
	Storage       *storage.Service
	Settings      *settings.Settings
	SecureCookies bool // true when base URL scheme is https
	LogFile       string // path to log file (empty = no file logging)
	CommandIndex  []cmdinfo.CommandInfo // cobra command tree for /admin/help pages
}
