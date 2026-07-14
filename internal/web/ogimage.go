package web

import (
	"fmt"
	"hash/fnv"
	"net/http"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/ogcard"
)

// OGImage serves the generated share card at /og.png on a site's subdomain or
// connected custom domain (wired in SubdomainRouter).
func (h *Handler) OGImage(w http.ResponseWriter, r *http.Request) {
	slug := extractSlug(r, h.cfg.Domain)
	var (
		site *domain.SiteAggregate
		err  error
	)
	if slug == "" {
		site, err = h.sites.GetSiteAggregateByCustomDomain(r.Context(), effectiveHost(r))
	} else {
		site, err = h.sites.GetSiteAggregateBySlug(r.Context(), slug)
	}
	if err != nil || site == nil {
		h.render.RenderError(w, http.StatusNotFound)
		return
	}
	h.writeOGImage(w, r, site)
}

// OGImagePath serves the card at /sites/{slug}/og.png for path-based routing
// (local dev, and anywhere wildcard subdomains aren't set up).
func (h *Handler) OGImagePath(w http.ResponseWriter, r *http.Request) {
	site, err := h.sites.GetSiteAggregateBySlug(r.Context(), r.PathValue("slug"))
	if err != nil || site == nil {
		h.render.RenderError(w, http.StatusNotFound)
		return
	}
	h.writeOGImage(w, r, site)
}

func (h *Handler) writeOGImage(w http.ResponseWriter, r *http.Request, site *domain.SiteAggregate) {
	// Only live sites have a public page for the card to front.
	if site.Status != domain.SiteStatusLive {
		h.render.RenderError(w, http.StatusNotFound)
		return
	}

	accent := h.siteAccentHex(site)
	footer := site.Slug + "." + h.cfg.Domain

	// ETag over everything that affects the pixels, so scrapers revalidate
	// cheaply and pick up edits when the site changes.
	etag := fmt.Sprintf(`"og-%d"`, hashOG(site.BusinessName, site.Tagline, site.Contact.Location, accent, site.UpdatedAt.String()))
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	png, err := ogcard.Render(ogcard.Card{
		BusinessName: site.BusinessName,
		Tagline:      site.Tagline,
		Location:     site.Contact.Location,
		Footer:       footer,
		AccentHex:    accent,
	})
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

// siteAccentHex resolves the card's accent colour: an owner's exact brand
// colour if set, else the chosen palette's representative swatch, else the
// default indigo.
func (h *Handler) siteAccentHex(site *domain.SiteAggregate) string {
	if bc := site.BrandColor; bc != "" {
		return bc
	}
	if hex, ok := paletteSwatchColors[site.Palette]; ok {
		return hex
	}
	return "#4F46E5"
}

func hashOG(parts ...string) uint32 {
	hsh := fnv.New32a()
	for _, p := range parts {
		hsh.Write([]byte(p))
		hsh.Write([]byte{0})
	}
	return hsh.Sum32()
}
