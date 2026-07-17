package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// SiteLoader is satisfied by *service.Sites — kept as an interface here so
// middleware doesn't need to import the whole service package's surface.
type SiteLoader interface {
	GetSiteAggregateBySlug(ctx context.Context, slug string) (*domain.SiteAggregate, error)
	GetSiteBySlug(ctx context.Context, slug string) (*domain.Site, error)
	ResolveSlugRedirect(ctx context.Context, oldSlug string) (string, bool, error)
}

// Ownership gates /dashboard/sites/{slug}/* routes so a user can only act on
// sites they own. It loads the full site once and stashes it in the request
// context so handlers don't have to fetch it again.
type Ownership struct {
	sites SiteLoader
}

func NewOwnership(sites SiteLoader) *Ownership {
	return &Ownership{sites: sites}
}

func (o *Ownership) RequireSiteOwner(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		site, err := o.sites.GetSiteAggregateBySlug(r.Context(), slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if site == nil {
			o.redirectRenamedOrNotFound(w, r, slug)
			return
		}
		// Same response for "not found" and "not yours" — don't leak existence.
		if site.OwnerUserID != UserID(r) {
			http.NotFound(w, r)
			return
		}
		next(w, withSite(r, site))
	}
}

// RequireSiteOwnerLight is RequireSiteOwner's lightweight counterpart: it
// checks ownership from just the site's own row, without loading
// GetSiteAggregateBySlug's full fan-out of related-table queries. Use this
// for routes whose handler only needs core fields (ID, Slug, ...) rather
// than the full aggregate.
func (o *Ownership) RequireSiteOwnerLight(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		site, err := o.sites.GetSiteBySlug(r.Context(), slug)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		if site == nil {
			o.redirectRenamedOrNotFound(w, r, slug)
			return
		}
		if site.OwnerUserID != UserID(r) {
			http.NotFound(w, r)
			return
		}
		next(w, withLightSite(r, site))
	}
}

// redirectRenamedOrNotFound handles a slug that doesn't resolve to any
// site: the owner may have renamed their site's address since this link was
// bookmarked/emailed, so it follows the same redirect the public site route
// uses before giving up with a 404.
func (o *Ownership) redirectRenamedOrNotFound(w http.ResponseWriter, r *http.Request, slug string) {
	if r.Method == http.MethodGet {
		if newSlug, ok, err := o.sites.ResolveSlugRedirect(r.Context(), slug); err == nil && ok {
			rest := strings.TrimPrefix(r.URL.Path, "/dashboard/sites/"+slug)
			target := "/dashboard/sites/" + newSlug + rest
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
	}
	http.NotFound(w, r)
}
