package ui

import (
	"net/http"
	"strings"
)

// FlashData holds a flash message and its severity.
type FlashData struct {
	Message string
	IsError bool
}

// SetFlash sets a flash message cookie (success/info severity).
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

// SetFlashError sets a flash message cookie with error severity.
func SetFlashError(w http.ResponseWriter, message string) {
	SetFlash(w, "error:"+message)
}

// GetFlash reads and clears the flash message cookie.
func GetFlash(w http.ResponseWriter, r *http.Request) *FlashData {
	cookie, err := r.Cookie("gosilo_flash")
	if err != nil {
		return nil
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
	val := cookie.Value
	if strings.HasPrefix(val, "error:") {
		return &FlashData{Message: strings.TrimPrefix(val, "error:"), IsError: true}
	}
	return &FlashData{Message: val}
}
