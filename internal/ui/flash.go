package ui

import "net/http"

// SetFlash sets a flash message cookie.
func SetFlash(w http.ResponseWriter, message string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "gosilo_flash",
		Value:    message,
		Path:     "/",
		MaxAge:   60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// GetFlash reads and clears the flash message cookie.
func GetFlash(w http.ResponseWriter, r *http.Request) string {
	cookie, err := r.Cookie("gosilo_flash")
	if err != nil {
		return ""
	}
	// Clear it.
	http.SetCookie(w, &http.Cookie{
		Name:     "gosilo_flash",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return cookie.Value
}
