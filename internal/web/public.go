package web

import (
	"encoding/xml"
	"fmt"
	"net/http"
)

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "home", map[string]any{"Templates": siteTemplates})
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
	Loc string `xml:"loc"`
}

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

// Sitemap serves /sitemap.xml, listing the marketing homepage and every live
// site's public URL for search engine discovery.
func (h *Handler) Sitemap(w http.ResponseWriter, r *http.Request) {
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
		set.URLs = append(set.URLs, sitemapURL{Loc: fmt.Sprintf("https://%s.%s/", s.Slug, h.cfg.Domain)})
	}

	body, err := xml.MarshalIndent(set, "", "  ")
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Write([]byte(xml.Header))
	w.Write(body)
}
