package web

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"rstash/internal/config"
	"rstash/internal/ui"
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
	snap := d.Settings.Load()
	links := make([]ui.CustomLinkView, 0, len(snap.CustomLinks))
	for _, l := range snap.CustomLinks {
		links = append(links, ui.CustomLinkView{
			Label:       l.Label,
			URL:         l.URL,
			Description: l.Description,
			External:    !strings.HasPrefix(l.URL, "/"),
		})
	}
	return ui.PageData{
		Title:            title,
		CurrentUser:      userInfo(CurrentUser(r)),
		CSRFToken:        CSRFToken(r),
		Flash:            ui.GetFlash(w, r),
		RegistrationMode: snap.RegistrationMode,
		Content:          content,
		ActiveNav:        activeNavFromPath(r.URL.Path),
		TOSUrl:           legalURL(snap.TOSMode, snap.TOSContent, "/legal/terms"),
		PrivacyUrl:       legalURL(snap.PrivacyMode, snap.PrivacyContent, "/legal/privacy"),
		Version:          config.Version,
		HasMailer:        d.Mailer != nil,
		SiteName:         snap.SiteName,
		CustomLinks:      links,
	}
}

// legalURL returns the URL for a legal page based on the mode and content settings.
func legalURL(mode, content, textPath string) string {
	switch mode {
	case "url":
		return content
	case "text":
		return textPath
	}
	return ""
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
	pd.OpenAbuseReports, _ = d.Repo.CountOpenAbuseReports(r.Context())
	return pd
}

// errorContent holds the data for the styled error page template.
type errorContent struct {
	Code    int
	Title   string
	Message string
	Detail  string
}

// renderError renders a styled error page with the given status code and message.
func (d *UIDeps) renderError(w http.ResponseWriter, r *http.Request, code int, title, message string) {
	w.WriteHeader(code)
	d.Renderer.Render(w, "error_page", d.pageData(w, r, title, &errorContent{
		Code:    code,
		Title:   title,
		Message: message,
	}))
}

// renderErrorDetail renders a styled error page with additional detail text.
func (d *UIDeps) renderErrorDetail(w http.ResponseWriter, r *http.Request, code int, title, message, detail string) {
	w.WriteHeader(code)
	d.Renderer.Render(w, "error_page", d.pageData(w, r, title, &errorContent{
		Code:    code,
		Title:   title,
		Message: message,
		Detail:  detail,
	}))
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

// ClientIPForStorage returns an IP string suitable for persistence, normalized
// per the log_client_ips setting:
//   - "enabled" (default): the raw IP is returned as-is.
//   - "hashed": SHA-256 over (today-UTC || IP), hex-encoded; still distinguishes
//     sessions/events on the same day but cannot be converted back to identity.
//     The daily salt rotation means cross-day correlation doesn't work either.
//   - "disabled": empty string.
func (d *UIDeps) ClientIPForStorage(r *http.Request) string {
	ip := ClientIP(r)
	if ip == "" {
		return ""
	}
	switch d.Settings.Load().LogClientIPs {
	case "disabled":
		return ""
	case "hashed":
		salt := time.Now().UTC().Format("2006-01-02")
		sum := sha256.Sum256([]byte(salt + "|" + ip))
		return hex.EncodeToString(sum[:])
	default:
		return ip
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
