package middleware

import (
	"net/http"
	"net/url"
	"strings"
)

const flashCookie = "_flash"

// Flash is a one-time message shown on the next page render, along with
// its severity so the UI can style it accordingly.
type Flash struct {
	Message string
	Level   string // "success", "warning", or "error"
}

// SetFlash queues a one-time success message, shown on the next page render.
func SetFlash(w http.ResponseWriter, msg string) {
	setFlash(w, msg, "success")
}

// SetFlashWarning queues a one-time warning message.
func SetFlashWarning(w http.ResponseWriter, msg string) {
	setFlash(w, msg, "warning")
}

// SetFlashError queues a one-time error message.
func SetFlashError(w http.ResponseWriter, msg string) {
	setFlash(w, msg, "error")
}

func setFlash(w http.ResponseWriter, msg, level string) {
	http.SetCookie(w, &http.Cookie{
		Name: flashCookie, Value: level + "|" + url.QueryEscape(msg), Path: "/",
		MaxAge: 30, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
}

// GetFlash reads and clears the flash cookie in one call.
func GetFlash(w http.ResponseWriter, r *http.Request) Flash {
	c, err := r.Cookie(flashCookie)
	if err != nil {
		return Flash{}
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Path: "/", MaxAge: -1})

	level, encoded, ok := strings.Cut(c.Value, "|")
	if !ok {
		// Legacy cookie value with no level prefix.
		msg, _ := url.QueryUnescape(c.Value)
		return Flash{Message: msg, Level: "success"}
	}
	msg, _ := url.QueryUnescape(encoded)
	return Flash{Message: msg, Level: level}
}
