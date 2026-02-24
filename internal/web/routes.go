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
	mux.HandleFunc("GET /settings", settingsHandler.Show)
	mux.HandleFunc("POST /settings/password", RequireCSRF(settingsHandler.ChangePassword))
	mux.HandleFunc("POST /settings/tokens/{token}/revoke", RequireCSRF(settingsHandler.RevokeToken))
	mux.HandleFunc("POST /settings/sessions/{token}/terminate", RequireCSRF(settingsHandler.TerminateOwnSession))

	// File browser.
	filesHandler := FilesHandler(deps)
	mux.HandleFunc("GET /files", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/files/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /files/search", filesHandler.Search)
	mux.HandleFunc("GET /files/{path...}", filesHandler.Browse)
	mux.HandleFunc("POST /files/delete", RequireCSRF(filesHandler.Delete))
	mux.HandleFunc("POST /files/upload", RequireCSRF(filesHandler.Upload))
	mux.HandleFunc("POST /files/bulk-delete", RequireCSRF(filesHandler.BulkDelete))
	mux.HandleFunc("POST /files/create-module", RequireCSRF(filesHandler.CreateModule))

	// Admin sub-pages.
	adminHandler := AdminHandler(deps)
	mux.HandleFunc("GET /admin", adminHandler.ShowDashboard)
	mux.HandleFunc("GET /admin/users", adminHandler.ShowUsers)
	mux.HandleFunc("GET /admin/settings", adminHandler.ShowSettings)
	mux.HandleFunc("GET /admin/invites", adminHandler.ShowInvites)
	mux.HandleFunc("GET /admin/audit", adminHandler.ShowAudit)
	mux.HandleFunc("GET /admin/oauth-test", adminHandler.ShowOAuthTest)
	mux.HandleFunc("POST /admin/users/create", RequireCSRF(adminHandler.CreateUser))
	mux.HandleFunc("POST /admin/users/{id}/delete", RequireCSRF(adminHandler.DeleteUser))
	mux.HandleFunc("POST /admin/users/{id}/quota", RequireCSRF(adminHandler.SetUserQuota))
	mux.HandleFunc("POST /admin/users/{id}/toggle-admin", RequireCSRF(adminHandler.ToggleAdmin))
	mux.HandleFunc("POST /admin/users/{id}/toggle-disabled", RequireCSRF(adminHandler.ToggleDisabled))
	mux.HandleFunc("GET /admin/users/{id}/sessions", adminHandler.UserSessions)
	mux.HandleFunc("GET /admin/users/{id}/activity", adminHandler.UserActivity)
	mux.HandleFunc("POST /admin/sessions/{token}/terminate", RequireCSRF(adminHandler.TerminateSession))
	mux.HandleFunc("POST /admin/users/{id}/terminate-all", RequireCSRF(adminHandler.TerminateAllSessions))
	mux.HandleFunc("POST /admin/settings", RequireCSRF(adminHandler.UpdateSettings))
	mux.HandleFunc("POST /admin/settings/{key}/reset", RequireCSRF(adminHandler.ResetSetting))
	mux.HandleFunc("POST /admin/invites", RequireCSRF(adminHandler.CreateInvite))
	mux.HandleFunc("POST /admin/invites/{code}/delete", RequireCSRF(adminHandler.DeleteInvite))

	return mux
}
