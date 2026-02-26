package web

import "net/http"

// OAuthRoutes returns an http.Handler with the minimal routes needed for
// OAuth consent flow: login, logout, and setup (first-user bootstrap).
func OAuthRoutes(deps *UIDeps) http.Handler {
	mux := http.NewServeMux()

	// Setup wizard (needed so first user can be created when CLI wasn't used).
	setupHandler := SetupHandler(deps)
	mux.HandleFunc("GET /setup", setupHandler.ShowSetup)
	mux.HandleFunc("POST /setup", setupHandler.DoSetup)

	// Auth (login/logout — needed for OAuth session).
	authHandler := AuthHandler(deps)
	mux.HandleFunc("GET /login", authHandler.ShowLogin)
	mux.HandleFunc("POST /login", authHandler.DoLogin)
	mux.HandleFunc("POST /logout", RequireCSRF(authHandler.DoLogout))

	return mux
}

// FullRoutes returns an http.Handler with all web UI routes (the full web experience).
func FullRoutes(deps *UIDeps) http.Handler {
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

	// Admin sub-pages — all protected by AdminGuard middleware.
	adminHandler := AdminHandler(deps)
	mux.HandleFunc("GET /admin", AdminGuard(adminHandler.ShowDashboard))
	mux.HandleFunc("GET /admin/users", AdminGuard(adminHandler.ShowUsers))
	mux.HandleFunc("GET /admin/settings", AdminGuard(adminHandler.ShowSettings))
	mux.HandleFunc("GET /admin/settings/{key}", AdminGuard(adminHandler.ShowSettingDetail))
	mux.HandleFunc("GET /admin/audit", AdminGuard(adminHandler.ShowAudit))
	logsH := LogsHandler(deps)
	mux.HandleFunc("GET /admin/logs", AdminGuard(logsH.ShowLogs))
	helpH := HelpHandler(deps)
	mux.HandleFunc("GET /admin/help", AdminGuard(helpH.ShowIndex))
	mux.HandleFunc("GET /admin/help/{command...}", AdminGuard(helpH.ShowCommand))
	mux.HandleFunc("GET /admin/oauth-test", AdminGuard(adminHandler.ShowOAuthTest))
	mux.HandleFunc("POST /admin/users/create", AdminGuard(RequireCSRF(adminHandler.CreateUser)))
	mux.HandleFunc("POST /admin/users/{id}/delete", AdminGuard(RequireCSRF(adminHandler.DeleteUser)))
	mux.HandleFunc("POST /admin/users/{id}/quota", AdminGuard(RequireCSRF(adminHandler.SetUserQuota)))
	mux.HandleFunc("POST /admin/users/{id}/toggle-admin", AdminGuard(RequireCSRF(adminHandler.ToggleAdmin)))
	mux.HandleFunc("POST /admin/users/{id}/toggle-disabled", AdminGuard(RequireCSRF(adminHandler.ToggleDisabled)))
	mux.HandleFunc("GET /admin/users/{id}/sessions", AdminGuard(adminHandler.UserSessions))
	mux.HandleFunc("GET /admin/users/{id}/activity", AdminGuard(adminHandler.UserActivity))
	mux.HandleFunc("POST /admin/sessions/{token}/terminate", AdminGuard(RequireCSRF(adminHandler.TerminateSession)))
	mux.HandleFunc("POST /admin/users/{id}/terminate-all", AdminGuard(RequireCSRF(adminHandler.TerminateAllSessions)))
	mux.HandleFunc("POST /admin/settings", AdminGuard(RequireCSRF(adminHandler.UpdateSettings)))
	mux.HandleFunc("POST /admin/settings/{key}/reset", AdminGuard(RequireCSRF(adminHandler.ResetSetting)))

	return mux
}

// Routes returns an http.Handler that serves all web UI routes.
// Deprecated: use FullRoutes instead.
func Routes(deps *UIDeps) http.Handler {
	return FullRoutes(deps)
}
