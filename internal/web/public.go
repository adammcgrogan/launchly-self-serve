package web

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"sync"
	"time"
)

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	_, loggedIn := h.auth.CheckUser(w, r)
	h.render.Render(w, "home", map[string]any{"Templates": siteTemplates, "LoggedIn": loggedIn})
}

func (h *Handler) Pricing(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "pricing", map[string]any{})
}

func (h *Handler) TemplatesPage(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "templates", map[string]any{"Templates": siteTemplates})
}

func (h *Handler) Privacy(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "privacy", map[string]any{})
}

func (h *Handler) Terms(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "terms", map[string]any{})
}

func (h *Handler) Help(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "help", map[string]any{})
}

func (h *Handler) HelpCustomDomain(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "help_custom_domain", map[string]any{})
}

func (h *Handler) HelpAddress(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "help_address", map[string]any{})
}

func (h *Handler) HelpSwitchTemplate(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "help_switch_template", map[string]any{})
}

func (h *Handler) HelpAppearance(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "help_appearance", map[string]any{})
}

// Robots serves /robots.txt, pointing crawlers at the sitemap.
func (h *Handler) Robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, "User-agent: *\nAllow: /\nSitemap: https://%s/sitemap.xml\n", h.cfg.Domain)
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// sitemapTTL is how long a rendered sitemap is served from cache before it's
// rebuilt from a fresh live-site scan. Search engines don't need
// second-fresh sitemaps, and this bounds the DB work a crawler can trigger
// on the (unauthenticated) /sitemap.xml endpoint.
const sitemapTTL = 5 * time.Minute

var sitemapCache struct {
	sync.Mutex
	body      []byte
	expiresAt time.Time
}

// Sitemap serves /sitemap.xml, listing the marketing homepage and every live
// site's public URL (with a lastmod hint) for search engine discovery. The
// rendered document is cached for sitemapTTL so repeated crawler hits don't
// each trigger a full live-site scan.
func (h *Handler) Sitemap(w http.ResponseWriter, r *http.Request) {
	sitemapCache.Lock()
	defer sitemapCache.Unlock()

	if sitemapCache.body != nil && time.Now().Before(sitemapCache.expiresAt) {
		h.writeSitemap(w, sitemapCache.body)
		return
	}

	sites, err := h.sites.ListLiveSites(r.Context())
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	set := sitemapURLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  []sitemapURL{{Loc: fmt.Sprintf("https://%s/", h.cfg.Domain)}},
	}
	for _, s := range sites {
		lastMod := s.UpdatedAt
		if lastMod.IsZero() && s.PublishedAt != nil {
			lastMod = *s.PublishedAt
		}
		u := sitemapURL{Loc: fmt.Sprintf("https://%s.%s/", s.Slug, h.cfg.Domain)}
		if !lastMod.IsZero() {
			u.LastMod = lastMod.UTC().Format("2006-01-02")
		}
		set.URLs = append(set.URLs, u)
	}

	body, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	full := append([]byte(xml.Header), body...)
	sitemapCache.body = full
	sitemapCache.expiresAt = time.Now().Add(sitemapTTL)
	h.writeSitemap(w, full)
}

func (h *Handler) writeSitemap(w http.ResponseWriter, body []byte) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write(body)
}
