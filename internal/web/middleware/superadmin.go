package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
)

const superadminSessionCookie = "_superadmin_session"

// Superadmin gates the read-mostly cross-account view behind a single
// shared password (an env var, like the old admin panel) — entirely
// separate from customer Supabase auth.
type Superadmin struct {
	password      string
	signingKey    string
	secureCookies bool
}

func NewSuperadmin(password, signingKey string, secureCookies bool) *Superadmin {
	return &Superadmin{password: password, signingKey: signingKey, secureCookies: secureCookies}
}

func (s *Superadmin) deriveToken(ctx string) string {
	mac := hmac.New(sha256.New, []byte(s.signingKey))
	mac.Write([]byte(ctx))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Superadmin) CheckPassword(pw string) bool {
	return subtle.ConstantTimeCompare([]byte(pw), []byte(s.password)) == 1
}

func (s *Superadmin) SetSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: superadminSessionCookie, Value: s.deriveToken("superadmin-session-v1"), Path: "/superadmin",
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 86400 * 7,
	})
}

func (s *Superadmin) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: superadminSessionCookie, Path: "/superadmin", MaxAge: -1})
}

func (s *Superadmin) IsAuthenticated(r *http.Request) bool {
	c, err := r.Cookie(superadminSessionCookie)
	if err != nil {
		return false
	}
	return hmac.Equal([]byte(c.Value), []byte(s.deriveToken("superadmin-session-v1")))
}

func (s *Superadmin) RequireSuperadmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.IsAuthenticated(r) {
			http.Redirect(w, r, "/superadmin/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}
