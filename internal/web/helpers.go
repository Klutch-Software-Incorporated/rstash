package web

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	"gosilo/internal/ui"
)

// formatBytes returns a human-readable string for a byte count.
func formatBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// pageData builds a ui.PageData with the standard fields (CurrentUser, CSRFToken,
// Flash, RegistrationMode) populated from the request context and config.
func (d *UIDeps) pageData(w http.ResponseWriter, r *http.Request, title string, content any) ui.PageData {
	return ui.PageData{
		Title:            title,
		CurrentUser:      userInfo(CurrentUser(r)),
		CSRFToken:        CSRFToken(r),
		Flash:            ui.GetFlash(w, r),
		RegistrationMode: d.Settings.Load().RegistrationMode,
		Content:          content,
		ActiveNav:        activeNavFromPath(r.URL.Path),
	}
}

// activeNavFromPath returns the nav identifier based on the request path.
func activeNavFromPath(path string) string {
	// Handle /~{username}/... paths.
	if strings.HasPrefix(path, "/~") {
		// Find the sub-path after /~username/
		idx := strings.Index(path[2:], "/")
		if idx < 0 {
			return "home"
		}
		sub := path[2+idx+1:] // everything after /~username/
		switch {
		case sub == "" || sub == "/":
			return "home"
		case strings.HasPrefix(sub, "files"):
			return "files"
		case strings.HasPrefix(sub, "settings"):
			return "settings"
		default:
			return "home"
		}
	}

	switch {
	case path == "/":
		return "home"
	case strings.HasPrefix(path, "/files"):
		return "files"
	case strings.HasPrefix(path, "/settings"):
		return "settings"
	case strings.HasPrefix(path, "/admin"):
		return "admin"
	default:
		return ""
	}
}

// adminPageData builds a ui.PageData for admin sub-pages, setting ActiveAdminNav.
func (d *UIDeps) adminPageData(w http.ResponseWriter, r *http.Request, title string, nav string, content any) ui.PageData {
	pd := d.pageData(w, r, title, content)
	pd.ActiveAdminNav = nav
	pd.JSONApiEnabled = d.Settings.Load().JSONApi == "admin"
	return pd
}

// ClientIP extracts the client IP address from the request.
// It checks X-Forwarded-For (first entry), X-Real-IP, then RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			return ip
		}
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return strings.TrimSpace(rip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// validatePassword checks that a password meets minimum requirements.
// Returns an error message, or empty string if valid.
func validatePassword(password string) string {
	if len(password) < 8 {
		return "Password must be at least 8 characters."
	}
	return ""
}
