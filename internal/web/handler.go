// Package web is the HTTP layer: thin handlers that call internal/service
// and never touch the database or Supabase directly.
package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/config"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

type Handler struct {
	cfg    *config.Config
	store  *postgres.Store
	render *Renderer

	auth       *middleware.Auth
	superadmin *middleware.Superadmin
	ownership  *middleware.Ownership
	csrf       *middleware.CSRF

	accounts  *service.Accounts
	sites     *service.Sites
	billing   *service.Billing
	leads     *service.Leads
	analytics *service.Analytics
	cron      *service.Cron
	domains   *service.Domains

	loginLimiter   *middleware.RateLimiter
	signupLimiter  *middleware.RateLimiter
	contactLimiter *middleware.RateLimiter
}

// Deps bundles everything main.go constructs so the Handler constructor
// doesn't take an unwieldy parameter list.
type Deps struct {
	Cfg   *config.Config
	Store *postgres.Store

	Accounts  *service.Accounts
	Sites     *service.Sites
	Billing   *service.Billing
	Leads     *service.Leads
	Analytics *service.Analytics
	Cron      *service.Cron
	Domains   *service.Domains

	Auth       *middleware.Auth
	Superadmin *middleware.Superadmin
}

func New(d Deps) (*Handler, error) {
	h := &Handler{
		cfg:            d.Cfg,
		store:          d.Store,
		auth:           d.Auth,
		superadmin:     d.Superadmin,
		ownership:      middleware.NewOwnership(d.Sites),
		csrf:           middleware.NewCSRF(d.Cfg.CookieSigningKey),
		accounts:       d.Accounts,
		sites:          d.Sites,
		billing:        d.Billing,
		leads:          d.Leads,
		analytics:      d.Analytics,
		cron:           d.Cron,
		domains:        d.Domains,
		loginLimiter:   middleware.NewRateLimiter(10, 15*time.Minute),
		signupLimiter:  middleware.NewRateLimiter(5, 15*time.Minute),
		contactLimiter: middleware.NewRateLimiter(5, time.Minute),
	}

	h.render = NewRenderer()
	if err := h.render.LoadAll(siteTemplates); err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	return h, nil
}

// HealthCheck returns 200 if the database is reachable, 503 otherwise.
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	if err := h.store.Ping(); err != nil {
		http.Error(w, "db unavailable", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// checkCSRF verifies the request's csrf_token form field against the token
// derived for subject (a user ID string, or "superadmin"), using a
// constant-time comparison. On failure it writes the 403 response itself —
// callers should return immediately when this returns false.
func (h *Handler) checkCSRF(w http.ResponseWriter, r *http.Request, subject string) bool {
	if h.csrf.Verify(subject, r.FormValue("csrf_token")) {
		return true
	}
	http.Error(w, "invalid csrf token", http.StatusForbidden)
	return false
}

// baseURL returns the scheme+host for the current request.
func (h *Handler) baseURL(reqHost string) string {
	scheme := "https"
	if strings.Contains(reqHost, ":") {
		scheme = "http"
	}
	return scheme + "://" + reqHost
}

// siteURL builds the public subdomain URL for a site.
func (h *Handler) siteURL(slug string) string {
	return "https://" + slug + "." + h.cfg.Domain
}

// secureCookies reports whether cookies should carry the Secure flag —
// false only for local dev over plain HTTP.
func (h *Handler) secureCookies() bool {
	return !strings.Contains(h.cfg.Domain, "localhost")
}
