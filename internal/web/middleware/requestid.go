package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestIDHeader is the header a request ID is read from (if a proxy set
// one) and always echoed back on, so a client or upstream can correlate a
// response with the server logs.
const RequestIDHeader = "X-Request-ID"

// RequestID assigns each request a short ID — reusing an inbound
// X-Request-ID when a trusted proxy already set one — stores it in the
// request context, and echoes it back in the response header, so a user's
// report ("I saw an error at 14:03, request abc123") can be tied to the
// structured log line for that request.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDCtxKey, id)))
	})
}

// GetRequestID returns the request ID assigned by the RequestID middleware,
// or "" if it hasn't run on this request.
func GetRequestID(r *http.Request) string {
	id, _ := r.Context().Value(requestIDCtxKey).(string)
	return id
}

func newRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}
