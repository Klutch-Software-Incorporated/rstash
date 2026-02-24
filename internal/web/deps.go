package web

import (
	"database/sql"

	"gosilo/internal/auth"
	"gosilo/internal/config"
	"gosilo/internal/settings"
	"gosilo/internal/storage"
	"gosilo/internal/ui"
)

// UIDeps holds the dependencies needed by UI handlers.
type UIDeps struct {
	Auth     auth.Service
	DB       *sql.DB // still needed for: node stats, oauth tokens, activity feed
	Renderer *ui.Renderer
	Config   *config.Config
	Storage  *storage.Service
	Settings *settings.Settings
}
