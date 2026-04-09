package web

import (
	"net/http"
	"strings"
)

// FullRoutes returns an http.Handler with all web UI routes (the full web experience).
func FullRoutes(deps *UIDeps) http.Handler {
	mux := http.NewServeMux()

	// Home page: redirect logged-in users to their profile, show landing for guests.
	homeH := HomeHandler(deps)
	mux.HandleFunc("GET /", homeH.Show)

	// Setup wizard.
	setupHandler := SetupHandler(deps)
	mux.HandleFunc("GET /setup", setupHandler.ShowSetup)
	mux.HandleFunc("POST /setup", RequireCSRF(setupHandler.DoSetup))

	// Auth (login/logout).
	authHandler := AuthHandler(deps)
	mux.HandleFunc("GET /login", authHandler.ShowLogin)
	mux.HandleFunc("POST /login", RequireCSRF(authHandler.DoLogin))
	mux.HandleFunc("POST /logout", RequireCSRF(authHandler.DoLogout))

	// Registration.
	registerHandler := RegisterHandler(deps)
	mux.HandleFunc("GET /register", registerHandler.ShowRegister)
	mux.HandleFunc("POST /register", RequireCSRF(registerHandler.DoRegister))

	// Account management (add email, verification, password reset).
	accountH := AccountHandler(deps)
	mux.HandleFunc("GET /account/email", accountH.ShowAddEmail)
	mux.HandleFunc("POST /account/email", RequireCSRF(accountH.DoAddEmail))
	mux.HandleFunc("GET /verify-email", accountH.VerifyEmail)
	mux.HandleFunc("POST /account/resend-verification", RequireCSRF(accountH.ResendVerification))
	mux.HandleFunc("GET /forgot-password", accountH.ShowForgotPassword)
	mux.HandleFunc("POST /forgot-password", RequireCSRF(accountH.DoForgotPassword))
	mux.HandleFunc("GET /reset-password", accountH.ShowResetPassword)
	mux.HandleFunc("POST /reset-password", RequireCSRF(accountH.DoResetPassword))

	// Legal pages (public, no auth).
	legalH := LegalHandler(deps)
	mux.HandleFunc("GET /legal/terms", legalH.ShowTerms)
	mux.HandleFunc("GET /legal/privacy", legalH.ShowPrivacy)
	mux.HandleFunc("GET /legal/licenses", legalH.ShowLicenses)

	// Abuse reporting (public, no auth).
	abuseH := AbuseHandler(deps)
	mux.HandleFunc("GET /abuse/report", abuseH.ShowReportForm)
	mux.HandleFunc("POST /abuse/report", RequireCSRF(abuseH.DoReport))

	// Admin sub-pages — all protected by AdminGuard middleware.
	adminHandler := AdminHandler(deps)
	mux.HandleFunc("GET /admin", AdminGuard(adminHandler.ShowDashboard))
	mux.HandleFunc("GET /admin/users", AdminGuard(adminHandler.ShowUsers))
	mux.HandleFunc("GET /admin/settings", AdminGuard(adminHandler.ShowSettings))
	mux.HandleFunc("GET /admin/settings/{key}", AdminGuard(adminHandler.ShowSettingDetail))
	mux.HandleFunc("GET /admin/audit", AdminGuard(adminHandler.ShowAudit))
	logsH := LogsHandler(deps)
	mux.HandleFunc("GET /admin/logs", AdminGuard(logsH.ShowLogs))
	mux.HandleFunc("GET /admin/status", AdminGuard(adminHandler.ShowStatus))
	mux.HandleFunc("GET /admin/email", AdminGuard(adminHandler.ShowEmail))
	mux.HandleFunc("POST /admin/email/test", AdminGuard(RequireCSRF(adminHandler.DoTestEmail)))
	mux.HandleFunc("POST /admin/email/announce", AdminGuard(RequireCSRF(adminHandler.DoAnnouncement)))
	mux.HandleFunc("GET /admin/oauth-test", AdminGuard(adminHandler.ShowOAuthTest))
	mux.HandleFunc("POST /admin/users/create", AdminGuard(RequireCSRF(adminHandler.CreateUser)))
	mux.HandleFunc("POST /admin/users/{id}/delete", AdminGuard(RequireCSRF(adminHandler.DeleteUser)))
	mux.HandleFunc("POST /admin/users/{id}/quota", AdminGuard(RequireCSRF(adminHandler.SetUserQuota)))
	mux.HandleFunc("POST /admin/users/{id}/toggle-admin", AdminGuard(RequireCSRF(adminHandler.ToggleAdmin)))
	mux.HandleFunc("POST /admin/users/{id}/toggle-disabled", AdminGuard(RequireCSRF(adminHandler.ToggleDisabled)))
	mux.HandleFunc("POST /admin/users/{id}/approve", AdminGuard(RequireCSRF(adminHandler.ApproveUser)))
	mux.HandleFunc("POST /admin/users/{id}/reject", AdminGuard(RequireCSRF(adminHandler.RejectUser)))
	mux.HandleFunc("POST /admin/settings", AdminGuard(RequireCSRF(adminHandler.UpdateSettings)))
	mux.HandleFunc("POST /admin/settings/{key}/reset", AdminGuard(RequireCSRF(adminHandler.ResetSetting)))
	mux.HandleFunc("GET /admin/abuse", AdminGuard(abuseH.ShowAbuseReports))
	mux.HandleFunc("POST /admin/abuse/{id}/review", AdminGuard(RequireCSRF(abuseH.ReviewAbuseReport)))

	// Profile routes are handled via a custom router since Go's ServeMux
	// doesn't support partial-segment wildcards like /~{username}/.
	tildeRouter := profileRouter(deps)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/~") {
			tildeRouter.ServeHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// profileRouter builds a handler that dispatches /~{username}/... requests.
// It manually parses the username from the URL path and sets it via SetPathValue,
// then delegates to UserScopeGuard-wrapped profile handlers.
func profileRouter(deps *UIDeps) http.Handler {
	ph := ProfileHandler(deps)
	scope := UserScopeGuard(deps.Auth)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse /~{username}/{subPath...}
		rest := strings.TrimPrefix(r.URL.Path, "/~")
		slashIdx := strings.Index(rest, "/")
		var username, sub string
		if slashIdx < 0 {
			username = rest
			sub = ""
		} else {
			username = rest[:slashIdx]
			sub = rest[slashIdx:] // includes leading /
		}

		if username == "" {
			http.NotFound(w, r)
			return
		}

		// Set path values so UserScopeGuard and handlers can read them.
		r.SetPathValue("username", username)

		method := r.Method

		// Dispatch based on method and sub-path.
		switch {
		// --- GET routes ---
		case method == "GET" && (sub == "/" || sub == ""):
			scope(ph.ShowDashboard)(w, r)
		case method == "GET" && sub == "/settings":
			scope(ph.ShowSettings)(w, r)
		case method == "GET" && sub == "/files/search":
			scope(ph.SearchFiles)(w, r)
		case method == "GET" && strings.HasPrefix(sub, "/files/"):
			r.SetPathValue("path", strings.TrimPrefix(sub, "/files/"))
			scope(ph.BrowseFiles)(w, r)
		case method == "GET" && sub == "/files":
			// Redirect /~user/files to /~user/files/
			http.Redirect(w, r, r.URL.Path+"/", http.StatusMovedPermanently)

		// --- POST routes: file operations ---
		case method == "POST" && sub == "/files/delete":
			scope(RequireCSRF(ph.DeleteFile))(w, r)
		case method == "POST" && sub == "/files/bulk-delete":
			scope(RequireCSRF(ph.BulkDeleteFiles))(w, r)

		// --- POST routes: settings operations ---
		case method == "POST" && sub == "/settings/email":
			scope(RequireCSRF(ph.ChangeEmail))(w, r)
		case method == "POST" && sub == "/settings/password":
			scope(RequireCSRF(ph.ChangePassword))(w, r)
		case method == "POST" && sub == "/settings/sessions/terminate-all":
			scope(RequireCSRF(ph.TerminateAllSessions))(w, r)
		case method == "POST" && strings.HasPrefix(sub, "/settings/tokens/") && strings.HasSuffix(sub, "/revoke"):
			token := strings.TrimPrefix(sub, "/settings/tokens/")
			token = strings.TrimSuffix(token, "/revoke")
			r.SetPathValue("token", token)
			scope(RequireCSRF(ph.RevokeToken))(w, r)
		case method == "POST" && strings.HasPrefix(sub, "/settings/sessions/") && strings.HasSuffix(sub, "/terminate"):
			token := strings.TrimPrefix(sub, "/settings/sessions/")
			token = strings.TrimSuffix(token, "/terminate")
			if token != "" && token != "terminate-all" {
				r.SetPathValue("token", token)
				scope(RequireCSRF(ph.TerminateSession))(w, r)
			} else {
				http.NotFound(w, r)
			}

		// --- POST routes: admin actions on profile ---
		case method == "POST" && sub == "/admin/toggle-admin":
			scope(RequireCSRF(ph.ToggleAdmin))(w, r)
		case method == "POST" && sub == "/admin/approve":
			scope(RequireCSRF(ph.ApproveUser))(w, r)
		case method == "POST" && sub == "/admin/toggle-disabled":
			scope(RequireCSRF(ph.ToggleDisabled))(w, r)
		case method == "POST" && sub == "/admin/delete":
			scope(RequireCSRF(ph.DeleteUser))(w, r)
		case method == "POST" && sub == "/admin/quota":
			scope(RequireCSRF(ph.SetQuota))(w, r)

		default:
			http.NotFound(w, r)
		}
	})
}

