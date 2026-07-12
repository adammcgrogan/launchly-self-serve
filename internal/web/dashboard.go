package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// Dashboard lists every site the logged-in user owns.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	sites, err := h.sites.ListSitesByOwner(r.Context(), middleware.UserID(r))
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.render.Render(w, "dashboard:sites", map[string]any{
		"Sites":         sites,
		"Flash":         middleware.GetFlash(w, r),
		"EmailVerified": h.emailVerified(r),
	})
}

// emailVerified reports whether the logged-in user has confirmed their
// email, for the dashboard's unverified-email nudge banner. It fails open
// (treats lookup errors as verified) so a profile-lookup hiccup never blocks
// the dashboard from rendering.
func (h *Handler) emailVerified(r *http.Request) bool {
	profile, err := h.accounts.GetProfile(r.Context(), middleware.UserID(r))
	if err != nil || profile == nil {
		return true
	}
	return profile.EmailVerified
}

// SiteOverview shows one site's status, live URL, trial/billing state,
// stats, and recent leads, plus every site-level setting grouped into tabs.
// RequireSiteOwner has already loaded the site into the request context.
func (h *Handler) SiteOverview(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)

	if r.URL.Query().Get("launched") == "1" {
		siteURL := h.siteURL(site.Slug)
		h.render.Render(w, "dashboard:launched", map[string]any{
			"Site":          site,
			"SiteURL":       siteURL,
			"EmailVerified": h.emailVerified(r),
		})
		return
	}

	leads, err := h.leads.ListBySite(r.Context(), site.ID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	newLeadCount := 0
	for _, l := range leads {
		if l.Status == domain.LeadStatusNew {
			newLeadCount++
		}
	}
	since7 := time.Now().UTC().Add(-7 * 24 * time.Hour)
	stats, _ := h.analytics.GetSiteStats(r.Context(), site.ID, since7)
	allTimeStats, _ := h.analytics.GetSiteStats(r.Context(), site.ID, site.CreatedAt)

	tmpl, _ := findTemplate(site.TemplateID)

	domainData := map[string]any{
		"FallbackOrigin": h.domains.FallbackOrigin(),
		"IsPro":          site.Billing.Plan == domain.PlanPro,
	}
	if site.CustomDomain != "" && site.CustomDomainStatus == domain.CustomDomainPending {
		if hostname, err := h.domains.RefreshCustomDomainStatus(r.Context(), site.ID); err == nil {
			domainData["Hostname"] = hostname
			if hostname.Active() {
				site.CustomDomainStatus = domain.CustomDomainActive
			} else if hostname.Failed() {
				site.CustomDomainStatus = domain.CustomDomainFailed
			}
		}
	}

	h.render.Render(w, "dashboard:site", map[string]any{
		"Site":               site,
		"Leads":              leads,
		"LeadCount":          len(leads),
		"NewLeadCount":       newLeadCount,
		"Stats":              stats,
		"AllTimeStats":       allTimeStats,
		"SiteURL":            h.siteURL(site.Slug),
		"Flash":              middleware.GetFlash(w, r),
		"CSRFToken":          h.csrf.Token(middleware.UserID(r).String()),
		"Upgraded":           r.URL.Query().Get("upgraded") == "1",
		"SMSAlertsAvailable": h.cfg.SMSAlertsAvailable(),
		"EmailVerified":      h.emailVerified(r),

		"Design":           tmpl,
		"Templates":        siteTemplates,
		"Palettes":         tmpl.Palettes,
		"Socials":          socialLinksMap(site.SocialLinks),
		"ServicesText":     servicesToLines(site.Services),
		"CertsText":        certificationsToLines(site.Certifications),
		"TestimonialsText": testimonialsToLines(site.Testimonials),
		"GalleryText":      galleryToLines(site.GalleryImages),
		"HoursByDay":       businessHoursByDay(site.BusinessHours),
		"Weekdays":         weekdays,
		"Timezones":        timezones,
		"Domain":           h.cfg.Domain,
		"DomainData":       domainData,
	})
}

// Account shows the logged-in user's email and account-level actions
// (password reset goes through Supabase's own recovery email flow).
func (h *Handler) Account(w http.ResponseWriter, r *http.Request) {
	profile, err := h.accounts.GetProfile(r.Context(), middleware.UserID(r))
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.render.Render(w, "dashboard:account", map[string]any{
		"Profile":       profile,
		"Flash":         middleware.GetFlash(w, r),
		"EmailVerified": profile.EmailVerified,
		"CSRFToken":     h.csrf.Token(middleware.UserID(r).String()),
	})
}

// accountExportSite bundles a site's full aggregate with its leads for the
// account data export — leads aren't part of SiteAggregate since most
// callers (e.g. the site editor) don't need them alongside every field.
type accountExportSite struct {
	*domain.SiteAggregate
	Leads []domain.Lead `json:"leads"`
}

// ExportAccountData downloads everything this app stores about the logged-in
// user — their profile, every site they own, and each site's leads — as JSON.
func (h *Handler) ExportAccountData(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	profile, err := h.accounts.GetProfile(r.Context(), userID)
	if err != nil || profile == nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	sites, err := h.sites.ListSitesByOwner(r.Context(), userID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	export := struct {
		Profile *domain.Profile     `json:"profile"`
		Sites   []accountExportSite `json:"sites"`
	}{Profile: profile, Sites: []accountExportSite{}}
	for _, site := range sites {
		agg, err := h.sites.GetSiteAggregate(r.Context(), site.ID)
		if err != nil || agg == nil {
			h.render.RenderError(w, http.StatusInternalServerError)
			return
		}
		leads, err := h.leads.ListBySite(r.Context(), site.ID)
		if err != nil {
			h.render.RenderError(w, http.StatusInternalServerError)
			return
		}
		export.Sites = append(export.Sites, accountExportSite{SiteAggregate: agg, Leads: leads})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="launchly-account-data.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(export)
}

// DeleteAccount permanently deletes the logged-in user's account: any Stripe
// subscriptions on their sites are cancelled first (Stripe isn't reachable
// via the DB's cascading deletes), then the Supabase auth user is deleted,
// which cascades away the profile, sites, and everything hanging off them.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	if !h.checkCSRF(w, r, userID.String()) {
		return
	}
	sites, err := h.sites.ListSitesByOwner(r.Context(), userID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	for _, site := range sites {
		if err := h.billing.CancelSubscriptionIfActive(r.Context(), site.ID); err != nil {
			h.render.RenderError(w, http.StatusInternalServerError)
			return
		}
	}
	if err := h.accounts.DeleteAccount(r.Context(), userID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.auth.ClearSessionCookies(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handler) ExportLeads(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	leads, err := h.leads.ListBySite(r.Context(), site.ID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-leads.csv"`, site.Slug))
	cw := csv.NewWriter(w)
	cw.Write([]string{"Name", "Email", "Phone", "Service", "Preferred time", "Message", "Status", "Date"})
	for _, l := range leads {
		cw.Write([]string{l.Name, l.Email, l.Phone, l.ServiceLabel, l.PreferredTime, l.Message, string(l.Status), l.CreatedAt.Format("2006-01-02 15:04")})
	}
	cw.Flush()
}
