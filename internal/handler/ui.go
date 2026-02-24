package handler

import (
	"database/sql"
	"net/http"

	"gosilo/internal/config"
	"gosilo/internal/storage"
	"gosilo/internal/ui"
)

// UIDeps holds the dependencies needed by UI handlers.
type UIDeps struct {
	DB       *sql.DB
	Renderer *ui.Renderer
	Config   *config.Config
	Storage  *storage.Service
}

// UI returns an http.Handler that serves the web UI routes.
func UI(deps *UIDeps) http.Handler {
	mux := http.NewServeMux()

	// Home page.
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		user := CurrentUser(r)
		deps.Renderer.Render(w, "home", ui.PageData{
			Title:            "Gosilo",
			CurrentUser:      userInfo(user),
			CSRFToken:        CSRFToken(r),
			Flash:            GetFlash(w, r),
			RegistrationMode: deps.Config.RegistrationMode,
		})
	})

	// Setup wizard.
	setupHandler := SetupHandler(deps)
	mux.HandleFunc("GET /setup", setupHandler.ShowSetup)
	mux.HandleFunc("POST /setup", setupHandler.DoSetup)

	// Auth (login/logout).
	authHandler := AuthHandler(deps)
	mux.HandleFunc("GET /login", authHandler.ShowLogin)
	mux.HandleFunc("POST /login", authHandler.DoLogin)
	mux.HandleFunc("POST /logout", authHandler.DoLogout)

	// Registration.
	registerHandler := RegisterHandler(deps)
	mux.HandleFunc("GET /register", registerHandler.ShowRegister)
	mux.HandleFunc("POST /register", registerHandler.DoRegister)

	// Account settings.
	settingsHandler := SettingsHandler(deps)
	mux.HandleFunc("GET /settings", settingsHandler.ShowSettings)
	mux.HandleFunc("GET /settings/password", settingsHandler.ShowChangePassword)
	mux.HandleFunc("POST /settings/password", settingsHandler.ChangePassword)
	mux.HandleFunc("POST /settings/tokens/{token}/revoke", settingsHandler.RevokeToken)

	// File browser.
	filesHandler := FilesHandler(deps)
	mux.HandleFunc("GET /files", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/files/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /files/{path...}", filesHandler.Browse)
	mux.HandleFunc("POST /files/delete", filesHandler.Delete)

	// Admin (all routes require auth + admin, enforced inside handler).
	adminHandler := AdminHandler(deps)
	mux.HandleFunc("GET /admin", adminHandler.Dashboard)
	mux.HandleFunc("GET /admin/users", adminHandler.Users)
	mux.HandleFunc("POST /admin/users/{id}/delete", adminHandler.DeleteUser)
	mux.HandleFunc("GET /admin/invites", adminHandler.Invites)
	mux.HandleFunc("POST /admin/invites", adminHandler.CreateInvite)
	mux.HandleFunc("POST /admin/invites/{code}/delete", adminHandler.DeleteInvite)
	mux.HandleFunc("GET /admin/oauth-test", adminHandler.OAuthTest)

	return mux
}
