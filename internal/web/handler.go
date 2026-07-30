// Package web is the HTTP layer: thin handlers that call internal/service
// and never touch the database or Supabase directly.
package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/ai"
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

	accounts      *service.Accounts
	sites         *service.Sites
	billing       *service.Billing
	leads         *service.Leads
	analytics     *service.Analytics
	cron          *service.Cron
	domains       *service.Domains
	uploads       *service.Uploads
	members       *service.Members
	ai            *ai.Client
	superadminSvc *service.Superadmin
	analyticsQ    *analyticsQueue

	loginLimiter              *middleware.RateLimiter
	signupLimiter             *middleware.RateLimiter
	contactLimiter            *middleware.RateLimiter
	beaconLimiter             *middleware.RateLimiter
	resendVerificationLimiter *middleware.RateLimiter
	aiGenerateLimiter         *middleware.RateLimiter
	uploadLimiter             *middleware.RateLimiter
	domainLimiter             *middleware.RateLimiter
	inviteLimiter             *middleware.RateLimiter
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
	Uploads   *service.Uploads
	Members   *service.Members
	AI        *ai.Client

	Auth          *middleware.Auth
	Superadmin    *middleware.Superadmin
	SuperadminSvc *service.Superadmin
}

func New(d Deps) (*Handler, error) {
	h := &Handler{
		cfg:                       d.Cfg,
		store:                     d.Store,
		auth:                      d.Auth,
		superadmin:                d.Superadmin,
		superadminSvc:             d.SuperadminSvc,
		ownership:                 middleware.NewOwnership(d.Sites, d.Members),
		csrf:                      middleware.NewCSRF(d.Cfg.CookieSigningKey),
		accounts:                  d.Accounts,
		sites:                     d.Sites,
		billing:                   d.Billing,
		leads:                     d.Leads,
		analytics:                 d.Analytics,
		cron:                      d.Cron,
		domains:                   d.Domains,
		uploads:                   d.Uploads,
		members:                   d.Members,
		ai:                        d.AI,
		analyticsQ:                newAnalyticsQueue(),
		loginLimiter:              middleware.NewRateLimiter(10, 15*time.Minute),
		signupLimiter:             middleware.NewRateLimiter(5, 15*time.Minute),
		contactLimiter:            middleware.NewRateLimiter(5, time.Minute),
		beaconLimiter:             middleware.NewRateLimiter(20, time.Minute),
		resendVerificationLimiter: middleware.NewRateLimiter(5, 15*time.Minute),
		aiGenerateLimiter:         middleware.NewRateLimiter(10, time.Hour),
		uploadLimiter:             middleware.NewRateLimiter(60, time.Hour),
		domainLimiter:             middleware.NewRateLimiter(20, time.Hour),
		inviteLimiter:             middleware.NewRateLimiter(20, time.Hour),
	}

	h.render = NewRenderer(d.Cfg.Domain)
	if err := h.render.LoadAll(siteTemplates); err != nil {
		return nil, fmt.Errorf("load templates: %w", err)
	}
	return h, nil
}

// HealthCheck returns 200 if the database is reachable, 503 otherwise.
// RenderError renders the branded error page with the given HTTP status
// code. Exported so cmd/server can pass it to the panic-recovery middleware.
func (h *Handler) RenderError(w http.ResponseWriter, status int) {
	h.render.RenderError(w, status)
}

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
// derived for subject (a user ID string, or "superadmin") and sessionNonce
// (the Supabase session ID for customer routes, or "" for superadmin),
// using a constant-time comparison. On failure it writes the 403 response
// itself — callers should return immediately when this returns false.
func (h *Handler) checkCSRF(w http.ResponseWriter, r *http.Request, subject, sessionNonce string) bool {
	if h.csrf.Verify(subject, sessionNonce, r.FormValue("csrf_token")) {
		return true
	}
	http.Error(w, "invalid csrf token", http.StatusForbidden)
	return false
}

// CSRFTokenRefresh mints a fresh CSRF token for the current, still-valid
// session. CSRF tokens are embedded once into a rendered page and expire
// after csrfTokenTTL; a dashboard tab left open longer than that has a
// perfectly valid session but a stale token, so a save 403s with nothing the
// user can do about it. The client-side ajax-form handler calls this on a
// 403, splices the fresh token into the form, and retries the save once —
// this endpoint only ever reissues a token bound to the caller's own
// authenticated identity, it never lets a request mint a token for anyone
// else, so it doesn't weaken checkCSRF's expiry or session-binding checks.
// Requires the same session auth as other dashboard routes (wired via
// h.auth.RequireUser in router.go), so it can't be used unauthenticated.
func (h *Handler) CSRFTokenRefresh(w http.ResponseWriter, r *http.Request) {
	token := h.csrf.Token(middleware.UserID(r).String(), h.auth.SessionNonce(r))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"csrf_token": token})
}

// siteURL builds the public subdomain URL for a site.
func (h *Handler) siteURL(slug string) string {
	return "https://" + slug + "." + h.cfg.Domain
}
