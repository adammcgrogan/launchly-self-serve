package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
)

const superadminSessionCookie = "_superadmin_session"

// Superadmin gates the read-mostly cross-account view behind per-admin
// accounts (see #94) — entirely separate from customer Supabase auth. This
// type only handles the session cookie (who's currently logged in);
// checking an email/password pair against the superadmin_admins table is
// service.Superadmin's job, since that requires a DB round-trip and web
// isn't allowed to touch the database directly.
type Superadmin struct {
	signingKey    string
	secureCookies bool
}

func NewSuperadmin(signingKey string, secureCookies bool) *Superadmin {
	return &Superadmin{signingKey: signingKey, secureCookies: secureCookies}
}

// deriveToken binds the session token to the specific admin's email, so one
// admin's cookie can't be replayed as another's and audit log entries can
// attribute the action to whoever is actually signed in.
func (s *Superadmin) deriveToken(email string) string {
	mac := hmac.New(sha256.New, []byte(s.signingKey))
	mac.Write([]byte("superadmin-session-v2:" + email))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Superadmin) SetSession(w http.ResponseWriter, email string) {
	http.SetCookie(w, &http.Cookie{
		Name: superadminSessionCookie, Value: email + "|" + s.deriveToken(email), Path: "/superadmin",
		HttpOnly: true, Secure: s.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: 86400 * 7,
	})
}

func (s *Superadmin) ClearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: superadminSessionCookie, Path: "/superadmin", MaxAge: -1})
}

// CurrentAdmin returns the signed-in admin's email, or "" if the session
// cookie is missing or its signature doesn't match.
func (s *Superadmin) CurrentAdmin(r *http.Request) string {
	c, err := r.Cookie(superadminSessionCookie)
	if err != nil {
		return ""
	}
	email, sig, ok := strings.Cut(c.Value, "|")
	if !ok || !hmac.Equal([]byte(sig), []byte(s.deriveToken(email))) {
		return ""
	}
	return email
}

func (s *Superadmin) IsAuthenticated(r *http.Request) bool {
	return s.CurrentAdmin(r) != ""
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
