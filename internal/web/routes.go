package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

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

	// Home page: redirect logged-in users to their profile, show landing for guests.
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

	// --- Legacy redirects for old URL paths ---
	mux.HandleFunc("GET /settings", redirectToSelf("/settings"))
	mux.HandleFunc("GET /files", func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/~"+user.Username+"/files/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /files/search", redirectToSelf("/files/search"))
	mux.HandleFunc("GET /files/{path...}", func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		rawPath := r.PathValue("path")
		http.Redirect(w, r, "/~"+user.Username+"/files/"+rawPath, http.StatusMovedPermanently)
	})

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
	mux.HandleFunc("POST /admin/settings", AdminGuard(RequireCSRF(adminHandler.UpdateSettings)))
	mux.HandleFunc("POST /admin/settings/{key}/reset", AdminGuard(RequireCSRF(adminHandler.ResetSetting)))

	// Legacy admin user detail/session redirects.
	mux.HandleFunc("GET /admin/users/{id}/activity", AdminGuard(func(w http.ResponseWriter, r *http.Request) {
		username := adminIDToUsername(deps, r)
		if username == "" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/~"+username+"/", http.StatusMovedPermanently)
	}))
	mux.HandleFunc("GET /admin/users/{id}/sessions", AdminGuard(func(w http.ResponseWriter, r *http.Request) {
		username := adminIDToUsername(deps, r)
		if username == "" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, "/~"+username+"/settings", http.StatusMovedPermanently)
	}))

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

// Routes returns an http.Handler that serves all web UI routes.
// Deprecated: use FullRoutes instead.
func Routes(deps *UIDeps) http.Handler {
	return FullRoutes(deps)
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
		case method == "POST" && sub == "/files/upload":
			scope(RequireCSRF(ph.UploadFile))(w, r)
		case method == "POST" && sub == "/files/bulk-delete":
			scope(RequireCSRF(ph.BulkDeleteFiles))(w, r)
		case method == "POST" && sub == "/files/create-module":
			scope(RequireCSRF(ph.CreateModule))(w, r)
		case method == "POST" && sub == "/files/create-folder":
			scope(RequireCSRF(ph.CreateFolder))(w, r)

		// --- POST routes: settings operations ---
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

// redirectToSelf returns a handler that redirects to /~{username}/{suffix}.
func redirectToSelf(suffix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := CurrentUser(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		target := fmt.Sprintf("/~%s%s", user.Username, suffix)
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
	}
}

// adminIDToUsername resolves a user ID from {id} path value to a username.
func adminIDToUsername(deps *UIDeps, r *http.Request) string {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return ""
	}
	user, err := deps.Auth.GetUser(r.Context(), id)
	if err != nil || user == nil {
		return ""
	}
	return user.Username
}
