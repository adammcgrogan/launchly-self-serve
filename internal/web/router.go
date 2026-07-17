package web

import (
	"net/http"
	"strings"
)

// RegisterRoutes wires every route to its handler and middleware chain.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Health check (no auth — used by Railway).
	mux.HandleFunc("GET /healthz", h.HealthCheck)

	// Public marketing pages.
	mux.HandleFunc("GET /", h.Home)
	mux.HandleFunc("GET /pricing", h.Pricing)
	mux.HandleFunc("GET /templates", h.TemplatesPage)
	mux.HandleFunc("GET /privacy", h.Privacy)
	mux.HandleFunc("GET /terms", h.Terms)
	mux.HandleFunc("GET /help", h.Help)
	mux.HandleFunc("GET /help/custom-domain", h.HelpCustomDomain)
	mux.HandleFunc("GET /help/address", h.HelpAddress)
	mux.HandleFunc("GET /help/switch-template", h.HelpSwitchTemplate)
	mux.HandleFunc("GET /help/appearance", h.HelpAppearance)
	mux.HandleFunc("GET /robots.txt", h.Robots)
	mux.HandleFunc("GET /sitemap.xml", h.Sitemap)

	// Auth — no auth required to reach these, obviously.
	mux.HandleFunc("GET /signup", h.SignupForm)
	mux.HandleFunc("POST /signup", h.SignupSubmit)
	mux.HandleFunc("GET /login", h.LoginForm)
	mux.HandleFunc("POST /login", h.LoginSubmit)
	mux.HandleFunc("GET /logout", h.Logout)
	mux.HandleFunc("GET /forgot-password", h.ForgotPasswordForm)
	mux.HandleFunc("POST /forgot-password", h.ForgotPasswordSubmit)
	mux.HandleFunc("GET /reset-password", h.ResetPasswordForm)
	mux.HandleFunc("POST /reset-password", h.ResetPasswordSubmit)
	mux.HandleFunc("GET /resend-verification", h.ResendVerificationForm)
	mux.HandleFunc("POST /resend-verification", h.ResendVerificationSubmit)

	// Path-based site routing (works without wildcard subdomains — useful for local dev).
	mux.HandleFunc("GET /sites/{slug}", h.ServeSitePath)
	mux.HandleFunc("GET /sites/{slug}/og.png", h.OGImagePath)
	mux.HandleFunc("POST /sites/{slug}/contact", h.SubmitLeadPath)
	mux.HandleFunc("POST /sites/{slug}/e", h.RecordSiteEventPath)

	// Dashboard — every route requires a logged-in user; site-scoped routes
	// additionally require that user to own the site. Most handlers only
	// need the site's core fields (ID, Slug, ...), so they're wired through
	// the lightweight ownership check (owned); only handlers that render the
	// full aggregate (joined services/hours/gallery/etc.) use ownedFull.
	owned := func(next http.HandlerFunc) http.HandlerFunc {
		return h.auth.RequireUser(h.ownership.RequireSiteOwnerLight(next))
	}
	ownedFull := func(next http.HandlerFunc) http.HandlerFunc {
		return h.auth.RequireUser(h.ownership.RequireSiteOwner(next))
	}
	mux.HandleFunc("GET /dashboard", h.auth.RequireUser(h.Dashboard))
	mux.HandleFunc("GET /dashboard/account", h.auth.RequireUser(h.Account))
	mux.HandleFunc("GET /dashboard/account/export", h.auth.RequireUser(h.ExportAccountData))
	mux.HandleFunc("POST /dashboard/account/delete", h.auth.RequireUser(h.DeleteAccount))
	mux.HandleFunc("GET /dashboard/sites/new", h.auth.RequireUser(h.NewSiteForm))
	mux.HandleFunc("POST /dashboard/sites/new", h.auth.RequireUser(h.NewSiteSubmit))
	mux.HandleFunc("POST /dashboard/sites/new/generate-copy", h.auth.RequireUser(h.GenerateCopy))
	mux.HandleFunc("POST /dashboard/uploads", h.auth.RequireUser(h.UploadImage))
	mux.HandleFunc("GET /dashboard/sites/{slug}", ownedFull(h.SiteOverview))
	mux.HandleFunc("POST /dashboard/sites/{slug}/edit", owned(h.EditSubmit))
	mux.HandleFunc("POST /dashboard/sites/{slug}/appearance", owned(h.AppearanceSubmit))
	mux.HandleFunc("POST /dashboard/sites/{slug}/switch-template", owned(h.SwitchTemplateSubmit))
	mux.HandleFunc("POST /dashboard/sites/{slug}/form-type", owned(h.FormTypeSubmit))
	mux.HandleFunc("POST /dashboard/sites/{slug}/address", owned(h.AddressSubmit))
	mux.HandleFunc("POST /dashboard/sites/{slug}/domain", owned(h.DomainSubmit))
	mux.HandleFunc("POST /dashboard/sites/{slug}/domain/check", owned(h.DomainCheckStatus))
	mux.HandleFunc("POST /dashboard/sites/{slug}/domain/remove", owned(h.DomainRemove))
	mux.HandleFunc("POST /dashboard/sites/{slug}/publish", owned(h.PublishSite))
	mux.HandleFunc("POST /dashboard/sites/{slug}/unpublish", owned(h.UnpublishSite))
	mux.HandleFunc("POST /dashboard/sites/{slug}/delete", owned(h.DeleteSite))
	mux.HandleFunc("GET /dashboard/sites/{slug}/leads.csv", owned(h.ExportLeads))
	mux.HandleFunc("GET /dashboard/sites/{slug}/analytics.csv", owned(h.ExportAnalytics))
	mux.HandleFunc("GET /dashboard/sites/{slug}/analytics-card", owned(h.SiteAnalyticsCard))
	mux.HandleFunc("GET /dashboard/sites/{slug}/qr.png", owned(h.SiteQRCode))
	mux.HandleFunc("GET /dashboard/sites/{slug}/print", ownedFull(h.SitePrintPage))
	mux.HandleFunc("POST /dashboard/sites/{slug}/leads/{leadID}/status", owned(h.LeadStatusSubmit))
	mux.HandleFunc("POST /dashboard/sites/{slug}/announcement", owned(h.UpdateAnnouncement))
	mux.HandleFunc("POST /dashboard/sites/{slug}/analytics-frequency", owned(h.UpdateAnalyticsFrequency))
	mux.HandleFunc("POST /dashboard/sites/{slug}/notify-settings", owned(h.UpdateNotifySettings))
	mux.HandleFunc("POST /dashboard/sites/{slug}/send-analytics", owned(h.SendAnalyticsNow))
	mux.HandleFunc("POST /dashboard/sites/{slug}/tracking-settings", owned(h.UpdateTrackingSettings))
	mux.HandleFunc("POST /dashboard/sites/{slug}/upgrade", owned(h.UpgradeCheckout))
	mux.HandleFunc("POST /dashboard/sites/{slug}/cancel-subscription", owned(h.CancelSubscription))

	// Superadmin — shared-password session, separate from customer auth.
	mux.HandleFunc("GET /superadmin/login", h.SuperadminLoginForm)
	mux.HandleFunc("POST /superadmin/login", h.SuperadminLoginSubmit)
	mux.HandleFunc("GET /superadmin/logout", h.SuperadminLogout)
	mux.HandleFunc("GET /superadmin", h.superadmin.RequireSuperadmin(h.SuperadminDashboard))
	mux.HandleFunc("GET /superadmin/sites/{id}", h.superadmin.RequireSuperadmin(h.SuperadminSiteView))
	mux.HandleFunc("POST /superadmin/sites/{id}/edit", h.superadmin.RequireSuperadmin(h.SuperadminEditSubmit))
	mux.HandleFunc("POST /superadmin/sites/{id}/unpublish", h.superadmin.RequireSuperadmin(h.SuperadminUnpublish))
	mux.HandleFunc("POST /superadmin/sites/{id}/delete", h.superadmin.RequireSuperadmin(h.SuperadminDelete))

	// Stripe webhook — verified by signature, not session auth.
	mux.HandleFunc("POST /webhooks/stripe", h.StripeWebhook)
}

// SubdomainRouter routes subdomain requests to the public site handler and
// everything else to the main mux — the same pattern the old app used.
func SubdomainRouter(domain string, h *Handler, fallback http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(strings.Split(effectiveHost(r), ":")[0])
		isSubdomain := strings.HasSuffix(host, "."+domain)
		isLocalhost := host == "localhost" || host == "127.0.0.1"
		isMainDomain := host == domain || host == "www."+domain || isLocalhost
		isUnrecognizedHost := !isSubdomain && !isMainDomain && host != ""

		// An unrecognized host is either a site's connected custom domain or
		// an unrelated fallback host (e.g. a platform-assigned *.up.railway.app
		// URL) — only route it to ServeSite if it actually resolves to a site,
		// otherwise show the marketing site rather than 404ing. This uses the
		// lightweight site lookup since it only needs to know a site exists —
		// ServeSite loads the full aggregate itself once it takes over.
		if isUnrecognizedHost {
			site, err := h.sites.GetSiteByCustomDomain(r.Context(), host)
			if err != nil || site == nil {
				fallback.ServeHTTP(w, r)
				return
			}
		}
		isSiteDomain := isSubdomain || isUnrecognizedHost

		if isSiteDomain {
			if strings.HasPrefix(r.URL.Path, "/static/") {
				fallback.ServeHTTP(w, r)
				return
			}
			if r.Method == http.MethodGet && r.URL.Path == "/og.png" {
				h.OGImage(w, r)
				return
			}
			if r.Method == http.MethodPost && r.URL.Path == "/contact" {
				h.SubmitLead(w, r)
				return
			}
			if r.Method == http.MethodPost && r.URL.Path == "/e" {
				h.RecordSiteEvent(w, r)
				return
			}
			h.ServeSite(w, r)
			return
		}
		fallback.ServeHTTP(w, r)
	})
}
