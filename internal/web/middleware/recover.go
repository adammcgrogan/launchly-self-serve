package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover wraps next in panic recovery: it logs the panic value and stack
// trace via slog, then renders the branded error page instead of letting
// net/http silently abort the connection.
func Recover(renderError func(w http.ResponseWriter, status int), next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered",
					"error", err, "method", r.Method, "host", r.Host, "path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				renderError(w, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
