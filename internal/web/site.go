package web

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// ServeSite handles subdomain requests: slug.launchly.ltd.
func (h *Handler) ServeSite(w http.ResponseWriter, r *http.Request) {
	slug := extractSlug(r, h.cfg.Domain)
	h.serveSiteBySlug(w, r, slug, "/contact")
}

// ServeSitePath handles path-based requests (/sites/{slug}) — works
// everywhere including local dev, where wildcard subdomains aren't set up.
func (h *Handler) ServeSitePath(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	h.serveSiteBySlug(w, r, slug, "/sites/"+slug+"/contact")
}

func (h *Handler) serveSiteBySlug(w http.ResponseWriter, r *http.Request, slug, formAction string) {
	site, err := h.sites.GetSiteAggregateBySlug(r.Context(), slug)
	if err != nil || site == nil || site.Status != domain.SiteStatusLive {
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

// SubmitLead handles the contact form POST on subdomain-routed sites.
func (h *Handler) SubmitLead(w http.ResponseWriter, r *http.Request) {
	slug := extractSlug(r, h.cfg.Domain)
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
	); err != nil {
		slog.Error("submit lead", "slug", slug, "error", err)
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
