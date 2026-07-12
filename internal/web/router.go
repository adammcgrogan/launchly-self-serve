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
	mux.HandleFunc("POST /sites/{slug}/contact", h.SubmitLeadPath)

	// Dashboard — every route requires a logged-in user; site-scoped routes
	// additionally require that user to own the site.
	owned := func(next http.HandlerFunc) http.HandlerFunc {
		return h.auth.RequireUser(h.ownership.RequireSiteOwner(next))
	}
	mux.HandleFunc("GET /dashboard", h.auth.RequireUser(h.Dashboard))
	mux.HandleFunc("GET /dashboard/account", h.auth.RequireUser(h.Account))
	mux.HandleFunc("GET /dashboard/sites/new", h.auth.RequireUser(h.NewSiteForm))
	mux.HandleFunc("POST /dashboard/sites/new", h.auth.RequireUser(h.NewSiteSubmit))
	mux.HandleFunc("GET /dashboard/sites/{id}", owned(h.SiteOverview))
	mux.HandleFunc("GET /dashboard/sites/{id}/edit", owned(h.EditForm))
	mux.HandleFunc("POST /dashboard/sites/{id}/edit", owned(h.EditSubmit))
	mux.HandleFunc("GET /dashboard/sites/{id}/appearance", owned(h.AppearanceForm))
	mux.HandleFunc("POST /dashboard/sites/{id}/appearance", owned(h.AppearanceSubmit))
	mux.HandleFunc("GET /dashboard/sites/{id}/switch-template", owned(h.SwitchTemplateForm))
	mux.HandleFunc("POST /dashboard/sites/{id}/switch-template", owned(h.SwitchTemplateSubmit))
	mux.HandleFunc("GET /dashboard/sites/{id}/form-type", owned(h.FormTypeForm))
	mux.HandleFunc("POST /dashboard/sites/{id}/form-type", owned(h.FormTypeSubmit))
	mux.HandleFunc("GET /dashboard/sites/{id}/address", owned(h.AddressForm))
	mux.HandleFunc("POST /dashboard/sites/{id}/address", owned(h.AddressSubmit))
	mux.HandleFunc("GET /dashboard/sites/{id}/domain", owned(h.DomainForm))
	mux.HandleFunc("POST /dashboard/sites/{id}/domain", owned(h.DomainSubmit))
	mux.HandleFunc("POST /dashboard/sites/{id}/domain/check", owned(h.DomainCheckStatus))
	mux.HandleFunc("POST /dashboard/sites/{id}/domain/remove", owned(h.DomainRemove))
	mux.HandleFunc("POST /dashboard/sites/{id}/publish", owned(h.PublishSite))
	mux.HandleFunc("POST /dashboard/sites/{id}/unpublish", owned(h.UnpublishSite))
	mux.HandleFunc("POST /dashboard/sites/{id}/delete", owned(h.DeleteSite))
	mux.HandleFunc("GET /dashboard/sites/{id}/leads.csv", owned(h.ExportLeads))
	mux.HandleFunc("POST /dashboard/sites/{id}/announcement", owned(h.UpdateAnnouncement))
	mux.HandleFunc("POST /dashboard/sites/{id}/analytics-frequency", owned(h.UpdateAnalyticsFrequency))
	mux.HandleFunc("POST /dashboard/sites/{id}/notify-settings", owned(h.UpdateNotifySettings))
	mux.HandleFunc("POST /dashboard/sites/{id}/send-analytics", owned(h.SendAnalyticsNow))
	mux.HandleFunc("POST /dashboard/sites/{id}/upgrade", owned(h.UpgradeCheckout))
	mux.HandleFunc("POST /dashboard/sites/{id}/cancel-subscription", owned(h.CancelSubscription))

	// Superadmin — shared-password session, separate from customer auth.
	mux.HandleFunc("GET /superadmin/login", h.SuperadminLoginForm)
	mux.HandleFunc("POST /superadmin/login", h.SuperadminLoginSubmit)
	mux.HandleFunc("GET /superadmin/logout", h.SuperadminLogout)
	mux.HandleFunc("GET /superadmin", h.superadmin.RequireSuperadmin(h.SuperadminDashboard))
	mux.HandleFunc("GET /superadmin/sites/{id}", h.superadmin.RequireSuperadmin(h.SuperadminSiteView))
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
		isSiteDomain := isSubdomain || (!isMainDomain && host != "")

		if isSiteDomain {
			if strings.HasPrefix(r.URL.Path, "/static/") {
				fallback.ServeHTTP(w, r)
				return
			}
			if r.Method == http.MethodPost && r.URL.Path == "/contact" {
				h.SubmitLead(w, r)
				return
			}
			h.ServeSite(w, r)
			return
		}
		fallback.ServeHTTP(w, r)
	})
}
