package middleware

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/supabase"
	"github.com/google/uuid"
)

const (
	accessTokenCookie  = "sb_access_token"
	refreshTokenCookie = "sb_refresh_token"
	rememberMeCookie   = "sb_remember_me"
)

// Auth verifies Supabase-issued session cookies for customer-facing routes.
type Auth struct {
	jwtSecret     string
	supa          *supabase.Client
	secureCookies bool
}

func NewAuth(jwtSecret string, supa *supabase.Client, secureCookies bool) *Auth {
	return &Auth{jwtSecret: jwtSecret, supa: supa, secureCookies: secureCookies}
}

// SetSessionCookies stores a Supabase session as httpOnly cookies after
// signup/login (or a transparent refresh). When rememberMe is false, the
// refresh-token cookie is set without MaxAge so it's cleared when the
// browser closes instead of persisting for 30 days.
func (a *Auth) SetSessionCookies(w http.ResponseWriter, sess *supabase.Session, rememberMe bool) {
	maxAge := sess.ExpiresIn
	if maxAge <= 0 {
		maxAge = 3600
	}
	http.SetCookie(w, &http.Cookie{
		Name: accessTokenCookie, Value: sess.AccessToken, Path: "/",
		HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: maxAge,
	})
	refreshMaxAge := 0
	if rememberMe {
		refreshMaxAge = 60 * 60 * 24 * 30
	}
	http.SetCookie(w, &http.Cookie{
		Name: refreshTokenCookie, Value: sess.RefreshToken, Path: "/",
		HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: refreshMaxAge,
	})
	rememberValue := "0"
	if rememberMe {
		rememberValue = "1"
	}
	http.SetCookie(w, &http.Cookie{
		Name: rememberMeCookie, Value: rememberValue, Path: "/",
		HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteLaxMode, MaxAge: refreshMaxAge,
	})
}

func (a *Auth) ClearSessionCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: accessTokenCookie, Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: refreshTokenCookie, Path: "/", MaxAge: -1})
	http.SetCookie(w, &http.Cookie{Name: rememberMeCookie, Path: "/", MaxAge: -1})
}

// rememberMe reports whether the session should persist across browser
// restarts, based on the choice recorded at login. Missing cookie (e.g. a
// session established before this cookie existed) defaults to true to
// preserve prior always-30-days behavior.
func (a *Auth) rememberMe(r *http.Request) bool {
	c, err := r.Cookie(rememberMeCookie)
	if err != nil {
		return true
	}
	return c.Value == "1"
}

// AccessToken returns the raw access token cookie value, if present — used
// by the logout handler to invalidate the session on Supabase's side.
func (a *Auth) AccessToken(r *http.Request) string {
	c, err := r.Cookie(accessTokenCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// RequireUser verifies the access token cookie, transparently refreshing it
// via the refresh token cookie if expired, and stores the user ID in the
// request context. Redirects to /login if there's no valid session at all.
func (a *Auth) RequireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie(accessTokenCookie); err == nil {
			if claims, err := supabase.VerifyAccessToken(c.Value, a.jwtSecret); err == nil {
				next(w, withUserID(r, claims.UserID))
				return
			}
		}

		if rc, err := r.Cookie(refreshTokenCookie); err == nil {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			if sess, err := a.supa.RefreshSession(ctx, rc.Value); err == nil {
				a.SetSessionCookies(w, sess, a.rememberMe(r))
				next(w, withUserID(r, sess.UserID))
				return
			}
		}

		a.ClearSessionCookies(w)
		http.Redirect(w, r, "/login?next="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
	}
}

// CheckUser verifies the session cookies for pages that render differently
// for a logged-in visitor but don't require login. Unlike RequireUser, it
// never redirects — it just reports whether there's a valid session,
// silently refreshing an expired access token via the refresh cookie first.
func (a *Auth) CheckUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if c, err := r.Cookie(accessTokenCookie); err == nil {
		if claims, err := supabase.VerifyAccessToken(c.Value, a.jwtSecret); err == nil {
			return claims.UserID, true
		}
	}

	if rc, err := r.Cookie(refreshTokenCookie); err == nil {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		if sess, err := a.supa.RefreshSession(ctx, rc.Value); err == nil {
			a.SetSessionCookies(w, sess, a.rememberMe(r))
			return sess.UserID, true
		}
	}

	return uuid.UUID{}, false
}
