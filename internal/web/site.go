package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// ServeSite handles subdomain requests (slug.launchly.ltd) and, when the
// host doesn't match that pattern, falls back to a Pro site's connected
// custom domain.
func (h *Handler) ServeSite(w http.ResponseWriter, r *http.Request) {
	slug := extractSlug(r, h.cfg.Domain)
	if slug == "" {
		if site, err := h.sites.GetSiteAggregateByCustomDomain(r.Context(), effectiveHost(r)); err == nil && site != nil {
			h.renderSite(w, r, site, "/contact")
			return
		}
	}
	h.serveSiteBySlug(w, r, slug, "/contact", func(newSlug string) string {
		return "https://" + newSlug + "." + h.cfg.Domain + r.URL.Path
	})
}

// ServeSitePath handles path-based requests (/sites/{slug}) — works
// everywhere including local dev, where wildcard subdomains aren't set up.
func (h *Handler) ServeSitePath(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	h.serveSiteBySlug(w, r, slug, "/sites/"+slug+"/contact", func(newSlug string) string {
		return "/sites/" + newSlug
	})
}

// serveSiteBySlug renders the site for slug, or — if it was renamed away
// from — 301s to redirectURL(newSlug) so old links keep working.
func (h *Handler) serveSiteBySlug(w http.ResponseWriter, r *http.Request, slug, formAction string, redirectURL func(newSlug string) string) {
	site, err := h.sites.GetSiteAggregateBySlug(r.Context(), slug)
	if err != nil {
		h.render.RenderError(w, http.StatusNotFound)
		return
	}
	if site == nil {
		if newSlug, ok, err := h.sites.ResolveSlugRedirect(r.Context(), slug); err == nil && ok {
			http.Redirect(w, r, redirectURL(newSlug), http.StatusMovedPermanently)
			return
		}
		h.renderClaimOrError(w, slug)
		return
	}
	h.renderSite(w, r, site, formAction)
}

// renderSite renders an already-resolved site (by slug or custom domain).
func (h *Handler) renderSite(w http.ResponseWriter, r *http.Request, site *domain.SiteAggregate, formAction string) {
	if site.Status == domain.SiteStatusPaused {
		h.render.Render(w, "paused", map[string]any{"BusinessName": site.BusinessName})
		return
	}
	if site.Status != domain.SiteStatusLive {
		h.render.RenderError(w, http.StatusNotFound)
		return
	}
	go h.recordPageView(r, site.ID)

	tmplKey := "site:" + site.TemplateID
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.render.Render(w, tmplKey, map[string]any{
		"Site":           site,
		"LeadSent":       r.URL.Query().Get("lead") == "1",
		"FormAction":     formAction,
		"Socials":        socialLinksMap(site.SocialLinks),
		"UmamiScriptURL": h.cfg.UmamiScriptURL,
	})
}

// renderClaimOrError shows the "this subdomain is available" claim page for
// slugs that pass through slug normalization unchanged, pitching signup to
// warm, business-name-typing traffic. Anything that doesn't round-trip
// (junk hosts, IPs, etc.) falls back to the generic 404 so we never reflect
// arbitrary input into the page.
func (h *Handler) renderClaimOrError(w http.ResponseWriter, slug string) {
	if slug == "" || service.ToSlug(slug) != slug {
		h.render.RenderError(w, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	h.render.Render(w, "claim", map[string]any{
		"Slug":      slug,
		"SignupURL": "https://" + h.cfg.Domain + "/signup",
	})
}

func (h *Handler) recordPageView(r *http.Request, siteID int) {
	ua := r.Header.Get("User-Agent")
	if isBot(ua) {
		return
	}
	ref := r.Referer()
	if u, err := url.Parse(ref); err == nil && u.Host != "" {
		ref = u.Host
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	if err := h.analytics.RecordPageView(r.Context(), siteID, path, ref, middleware.ClientIP(r)); err != nil {
		slog.Error("record page view", "error", err)
	}
}

func isBot(ua string) bool {
	lower := strings.ToLower(ua)
	for _, pat := range []string{"bot", "crawler", "spider", "slurp", "wget", "curl", "python", "java/", "go-http", "libwww", "scrapy", "postman", "headless"} {
		if strings.Contains(lower, pat) {
			return true
		}
	}
	return ua == ""
}

// SubmitLead handles the contact form POST on subdomain-routed sites, and
// falls back to a Pro site's connected custom domain.
func (h *Handler) SubmitLead(w http.ResponseWriter, r *http.Request) {
	slug := extractSlug(r, h.cfg.Domain)
	if slug == "" {
		h.submitLeadForCustomDomain(w, r)
		return
	}
	h.submitLeadForSlug(w, r, slug, "/?lead=1")
}

// SubmitLeadPath handles the contact form POST on path-routed sites.
func (h *Handler) SubmitLeadPath(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	h.submitLeadForSlug(w, r, slug, "/sites/"+slug+"?lead=1")
}

func (h *Handler) submitLeadForSlug(w http.ResponseWriter, r *http.Request, slug, redirectURL string) {
	if !h.contactLimiter.Allow(middleware.ClientIP(r)) {
		http.Error(w, "Too many requests — please wait a moment and try again.", http.StatusTooManyRequests)
		return
	}
	site, err := h.sites.GetSiteAggregateBySlug(r.Context(), slug)
	if err != nil || site == nil || site.Status != domain.SiteStatusLive {
		http.NotFound(w, r)
		return
	}
	h.submitLeadForSite(w, r, site, redirectURL)
}

func (h *Handler) submitLeadForCustomDomain(w http.ResponseWriter, r *http.Request) {
	if !h.contactLimiter.Allow(middleware.ClientIP(r)) {
		http.Error(w, "Too many requests — please wait a moment and try again.", http.StatusTooManyRequests)
		return
	}
	site, err := h.sites.GetSiteAggregateByCustomDomain(r.Context(), effectiveHost(r))
	if err != nil || site == nil || site.Status != domain.SiteStatusLive {
		http.NotFound(w, r)
		return
	}
	h.submitLeadForSite(w, r, site, "/?lead=1")
}

// submitLeadForSite validates and saves a contact-form submission for an
// already-resolved, live site (by slug or custom domain).
func (h *Handler) submitLeadForSite(w http.ResponseWriter, r *http.Request, site *domain.SiteAggregate, redirectURL string) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Honeypot: silently succeed so bots don't know they were rejected.
	if r.FormValue("website") != "" {
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	if err := h.leads.SubmitLead(r.Context(), site.ID,
		name, strings.TrimSpace(r.FormValue("email")), strings.TrimSpace(r.FormValue("phone")), strings.TrimSpace(r.FormValue("message")),
		strings.TrimSpace(r.FormValue("service_label")), strings.TrimSpace(r.FormValue("preferred_time")),
	); err != nil {
		slog.Error("submit lead", "site_id", site.ID, "error", err)
		http.Error(w, "could not save lead", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

// extractSlug pulls the subdomain from the request host, e.g.
// "adam-barbers.launchly.ltd" → "adam-barbers".
func extractSlug(r *http.Request, domain string) string {
	host := effectiveHost(r)
	suffix := "." + domain
	if strings.HasSuffix(host, suffix) {
		return strings.TrimSuffix(host, suffix)
	}
	return ""
}

// effectiveHost returns X-Real-Host if set (e.g. from a proxy fronting
// wildcard subdomains), falling back to the raw Host header.
func effectiveHost(r *http.Request) string {
	host := r.Header.Get("X-Real-Host")
	if host == "" {
		host = r.Host
	}
	return strings.ToLower(strings.Split(host, ":")[0])
}
