package middleware

import (
	"net/http"
	"net/url"
)

const flashCookie = "_flash"

// SetFlash queues a one-time message, shown on the next page render.
func SetFlash(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{
		Name: flashCookie, Value: url.QueryEscape(msg), Path: "/",
		MaxAge: 30, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// GetFlash reads and clears the flash cookie in one call.
func GetFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie(flashCookie)
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Path: "/", MaxAge: -1})
	msg, _ := url.QueryUnescape(c.Value)
	return msg
}
