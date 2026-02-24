package web

import (
	"fmt"
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
		RegistrationMode: d.Config.RegistrationMode,
		Content:          content,
		ActiveNav:        activeNavFromPath(r.URL.Path),
	}
}

// activeNavFromPath returns the nav identifier based on the request path.
func activeNavFromPath(path string) string {
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

// validatePassword checks that a password meets minimum requirements.
// Returns an error message, or empty string if valid.
func validatePassword(password string) string {
	if len(password) < 8 {
		return "Password must be at least 8 characters."
	}
	return ""
}
