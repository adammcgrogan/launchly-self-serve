package web

import (
	"context"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
	"github.com/google/uuid"
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
	go h.recordPageView(r, site.ID, site.OwnerUserID)

	open, openLabel := site.OpenNow()

	tmplKey := "site:" + site.TemplateID
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	h.render.Render(w, tmplKey, map[string]any{
		"Site":        site,
		"LeadSent":    r.URL.Query().Get("lead") == "1",
		"FormAction":  formAction,
		"EventAction": strings.TrimSuffix(formAction, "/contact") + "/e",
		"Socials":     socialLinksMap(site.SocialLinks),
		"Open":        open,
		"OpenLabel":   openLabel,
		"JSONLD":      localBusinessJSONLD(site, h.siteURL(site.Slug)),
		"FAQJSONLD":   faqPageJSONLD(site),
	})
}

// JSON-LD types for the LocalBusiness structured data block emitted on
// every published site. Kept as marshalled Go structs rather than
// hand-assembled JSON in the template, which is fragile against quoting bugs.
type jsonLDAddress struct {
	Type            string `json:"@type"`
	StreetAddress   string `json:"streetAddress,omitempty"`
	AddressLocality string `json:"addressLocality,omitempty"`
}

type jsonLDOpeningHours struct {
	Type      string `json:"@type"`
	DayOfWeek string `json:"dayOfWeek"`
	Opens     string `json:"opens"`
	Closes    string `json:"closes"`
}

type jsonLDService struct {
	Type        string `json:"@type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type jsonLDOffer struct {
	Type        string        `json:"@type"`
	ItemOffered jsonLDService `json:"itemOffered"`
	Price       string        `json:"price,omitempty"`
}

type jsonLDOfferCatalog struct {
	Type            string        `json:"@type"`
	Name            string        `json:"name,omitempty"`
	ItemListElement []jsonLDOffer `json:"itemListElement"`
}

type jsonLDLocalBusiness struct {
	Context                   string                 `json:"@context"`
	Type                      string                 `json:"@type"`
	Name                      string                 `json:"name"`
	Description               string                 `json:"description,omitempty"`
	Telephone                 string                 `json:"telephone,omitempty"`
	Email                     string                 `json:"email,omitempty"`
	Image                     string                 `json:"image,omitempty"`
	URL                       string                 `json:"url"`
	Address                   *jsonLDAddress         `json:"address,omitempty"`
	HasMap                    string                 `json:"hasMap,omitempty"`
	OpeningHoursSpecification []jsonLDOpeningHours   `json:"openingHoursSpecification,omitempty"`
	SameAs                    []string               `json:"sameAs,omitempty"`
	HasOfferCatalog           *jsonLDOfferCatalog    `json:"hasOfferCatalog,omitempty"`
	AreaServed                []string               `json:"areaServed,omitempty"`
	AggregateRating           *jsonLDAggregateRating `json:"aggregateRating,omitempty"`
}

type jsonLDAggregateRating struct {
	Type        string `json:"@type"`
	RatingValue string `json:"ratingValue"`
	ReviewCount int    `json:"reviewCount,omitempty"`
	BestRating  string `json:"bestRating"`
}

// localBusinessJSONLD builds the site's LocalBusiness structured data. Only
// fields backed by genuine data are included — aggregateRating is emitted
// only when the owner has entered a real Google/Facebook rating, and there
// are no stored geo coordinates, so no geo property.
func localBusinessJSONLD(site *domain.SiteAggregate, siteURL string) template.JS {
	biz := jsonLDLocalBusiness{
		Context:     "https://schema.org",
		Type:        "LocalBusiness",
		Name:        site.BusinessName,
		Description: site.Tagline,
		Telephone:   site.Contact.Phone,
		Email:       site.Contact.Email,
		Image:       site.LogoURL,
		URL:         siteURL,
		HasMap:      site.Contact.MapURL,
	}

	if site.Contact.Address != "" || site.Contact.Location != "" {
		biz.Address = &jsonLDAddress{
			Type:            "PostalAddress",
			StreetAddress:   site.Contact.Address,
			AddressLocality: site.Contact.Location,
		}
	}

	for _, h := range site.OpenDays() {
		biz.OpeningHoursSpecification = append(biz.OpeningHoursSpecification, jsonLDOpeningHours{
			Type:      "OpeningHoursSpecification",
			DayOfWeek: h.Weekday.String(),
			Opens:     h.OpensAt,
			Closes:    h.ClosesAt,
		})
	}

	for _, l := range site.SocialLinks {
		if l.URL != "" {
			biz.SameAs = append(biz.SameAs, l.URL)
		}
	}

	var offers []jsonLDOffer
	for _, s := range site.Services {
		if s.Label != "" {
			offers = append(offers, jsonLDOffer{
				Type:        "Offer",
				ItemOffered: jsonLDService{Type: "Service", Name: s.Label, Description: s.Description},
				Price:       s.PriceText,
			})
		}
	}
	if len(offers) > 0 {
		biz.HasOfferCatalog = &jsonLDOfferCatalog{Type: "OfferCatalog", Name: "Services", ItemListElement: offers}
	}

	for _, a := range site.ServiceAreas {
		if a.Area != "" {
			biz.AreaServed = append(biz.AreaServed, a.Area)
		}
	}

	// aggregateRating is owner-entered (Google/Facebook rating typed into the
	// dashboard) rather than derived from on-site testimonials, so it's only
	// emitted when the owner has actually set a rating.
	if site.Reviews.HasBadge() {
		biz.AggregateRating = &jsonLDAggregateRating{
			Type:        "AggregateRating",
			RatingValue: site.Reviews.Rating,
			ReviewCount: site.Reviews.ReviewCount,
			BestRating:  "5",
		}
	}

	out, err := json.Marshal(biz)
	if err != nil {
		slog.Error("marshal local business json-ld", "site_id", site.ID, "error", err)
		return ""
	}
	return template.JS(out)
}

type jsonLDAnswer struct {
	Type string `json:"@type"`
	Text string `json:"text"`
}

type jsonLDQuestion struct {
	Type           string       `json:"@type"`
	Name           string       `json:"name"`
	AcceptedAnswer jsonLDAnswer `json:"acceptedAnswer"`
}

type jsonLDFAQPage struct {
	Context    string           `json:"@context"`
	Type       string           `json:"@type"`
	MainEntity []jsonLDQuestion `json:"mainEntity"`
}

// faqPageJSONLD builds an FAQPage structured data block from a site's FAQ
// items, emitted as its own <script> tag alongside the LocalBusiness block
// since FAQPage is a distinct top-level schema.org type. Returns "" when the
// site has no FAQ items, so the template can skip the tag entirely.
func faqPageJSONLD(site *domain.SiteAggregate) template.JS {
	if len(site.FAQItems) == 0 {
		return ""
	}
	page := jsonLDFAQPage{Context: "https://schema.org", Type: "FAQPage"}
	for _, f := range site.FAQItems {
		if f.Question == "" {
			continue
		}
		page.MainEntity = append(page.MainEntity, jsonLDQuestion{
			Type:           "Question",
			Name:           f.Question,
			AcceptedAnswer: jsonLDAnswer{Type: "Answer", Text: f.Answer},
		})
	}
	if len(page.MainEntity) == 0 {
		return ""
	}
	out, err := json.Marshal(page)
	if err != nil {
		slog.Error("marshal faq page json-ld", "site_id", site.ID, "error", err)
		return ""
	}
	return template.JS(out)
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

// recordPageView records a page view unless the visitor is the site's own
// logged-in owner checking or editing their live site — otherwise owners
// repeatedly opening their own site inflate the very numbers meant to prove
// the product's value.
func (h *Handler) recordPageView(r *http.Request, siteID int, ownerUserID uuid.UUID) {
	ua := r.Header.Get("User-Agent")
	if isBot(ua) {
		return
	}
	if uid, ok := h.auth.CurrentUserID(r); ok && uid == ownerUserID {
		return
	}
	ref := r.Referer()
	if u, err := url.Parse(ref); err == nil && u.Host != "" {
		ref = u.Host
	}
	if h.isSelfReferral(ref, r.Host) {
		ref = ""
	}
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.analytics.RecordPageView(ctx, siteID, path, ref, middleware.ClientIP(r)); err != nil {
		slog.Error("record page view", "error", err)
	}
}

// isSelfReferral reports whether ref is the site's own host (in-site
// navigation) or the platform domain — neither is a useful "top referrer"
// for the site owner.
func (h *Handler) isSelfReferral(ref, requestHost string) bool {
	if ref == "" {
		return false
	}
	if strings.EqualFold(ref, requestHost) {
		return true
	}
	platform := h.cfg.Domain
	return strings.EqualFold(ref, platform) || strings.EqualFold(ref, "www."+platform)
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

// siteEventKinds are the conversion kinds the beacon endpoint accepts —
// "lead" is deliberately excluded since that's recorded server-side by
// Leads.SubmitLead, not via a client-side tap.
var siteEventKinds = map[string]domain.EventKind{
	"call":       domain.EventKindCall,
	"whatsapp":   domain.EventKindWhatsApp,
	"directions": domain.EventKindDirections,
}

// RecordSiteEvent handles the navigator.sendBeacon conversion ping fired
// from tel:/WhatsApp/directions taps on subdomain-routed sites, and falls
// back to a Pro site's connected custom domain.
func (h *Handler) RecordSiteEvent(w http.ResponseWriter, r *http.Request) {
	slug := extractSlug(r, h.cfg.Domain)
	if slug == "" {
		site, err := h.sites.GetSiteAggregateByCustomDomain(r.Context(), effectiveHost(r))
		if err == nil && site != nil {
			h.recordSiteEvent(r, site)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	site, err := h.sites.GetSiteAggregateBySlug(r.Context(), slug)
	if err == nil && site != nil {
		h.recordSiteEvent(r, site)
	}
	w.WriteHeader(http.StatusNoContent)
}

// RecordSiteEventPath handles the beacon on path-routed sites.
func (h *Handler) RecordSiteEventPath(w http.ResponseWriter, r *http.Request) {
	site, err := h.sites.GetSiteAggregateBySlug(r.Context(), r.PathValue("slug"))
	if err == nil && site != nil {
		h.recordSiteEvent(r, site)
	}
	w.WriteHeader(http.StatusNoContent)
}

// recordSiteEvent validates and records a conversion beacon for an
// already-resolved site. Always responds 204 regardless of outcome —
// sendBeacon ignores the response, and there's nothing useful to tell a
// caller that isn't already filtered out server-side (bot, rate-limited,
// unknown kind).
func (h *Handler) recordSiteEvent(r *http.Request, site *domain.SiteAggregate) {
	if site.Status != domain.SiteStatusLive {
		return
	}
	if isBot(r.Header.Get("User-Agent")) || !h.beaconLimiter.Allow(middleware.ClientIP(r)) {
		return
	}
	if uid, ok := h.auth.CurrentUserID(r); ok && uid == site.OwnerUserID {
		return
	}
	kind, ok := siteEventKinds[r.URL.Query().Get("kind")]
	if !ok {
		return
	}
	if err := h.analytics.RecordEvent(r.Context(), site.ID, kind, middleware.ClientIP(r)); err != nil {
		slog.Error("record site event", "site_id", site.ID, "error", err)
	}
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
		h.siteURL(site.Slug),
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
