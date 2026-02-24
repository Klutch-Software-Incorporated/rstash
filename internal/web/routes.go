package web

import "net/http"

// Routes returns an http.Handler that serves the web UI routes.
func Routes(deps *UIDeps) http.Handler {
	mux := http.NewServeMux()

	// Home page.
	homeH := HomeHandler(deps)
	mux.HandleFunc("GET /", homeH.Show)

	// Setup wizard.
	setupHandler := SetupHandler(deps)
	mux.HandleFunc("GET /setup", setupHandler.ShowSetup)
	mux.HandleFunc("POST /setup", setupHandler.DoSetup)

	// Auth (login/logout).
	authHandler := AuthHandler(deps)
	mux.HandleFunc("GET /login", authHandler.ShowLogin)
	mux.HandleFunc("POST /login", authHandler.DoLogin)
	mux.HandleFunc("POST /logout", RequireCSRF(authHandler.DoLogout))

	// Registration.
	registerHandler := RegisterHandler(deps)
	mux.HandleFunc("GET /register", registerHandler.ShowRegister)
	mux.HandleFunc("POST /register", registerHandler.DoRegister)

	// Account settings.
	settingsHandler := SettingsHandler(deps)
	mux.HandleFunc("GET /settings", settingsHandler.ShowSettings)
	mux.HandleFunc("GET /settings/password", settingsHandler.ShowChangePassword)
	mux.HandleFunc("POST /settings/password", RequireCSRF(settingsHandler.ChangePassword))
	mux.HandleFunc("POST /settings/tokens/{token}/revoke", RequireCSRF(settingsHandler.RevokeToken))

	// File browser.
	filesHandler := FilesHandler(deps)
	mux.HandleFunc("GET /files", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/files/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /files/{path...}", filesHandler.Browse)
	mux.HandleFunc("POST /files/delete", RequireCSRF(filesHandler.Delete))

	// Admin (all routes require auth + admin, enforced inside handler).
	adminHandler := AdminHandler(deps)
	mux.HandleFunc("GET /admin", adminHandler.Dashboard)
	mux.HandleFunc("GET /admin/users", adminHandler.Users)
	mux.HandleFunc("POST /admin/users/{id}/delete", RequireCSRF(adminHandler.DeleteUser))
	mux.HandleFunc("POST /admin/users/{id}/quota", RequireCSRF(adminHandler.SetUserQuota))
	mux.HandleFunc("GET /admin/invites", adminHandler.Invites)
	mux.HandleFunc("POST /admin/invites", RequireCSRF(adminHandler.CreateInvite))
	mux.HandleFunc("POST /admin/invites/{code}/delete", RequireCSRF(adminHandler.DeleteInvite))
	mux.HandleFunc("GET /admin/oauth-test", adminHandler.OAuthTest)

	return mux
}
