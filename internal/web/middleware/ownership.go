package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
)

// SiteLoader is satisfied by *service.Sites — kept as an interface here so
// middleware doesn't need to import the whole service package's surface.
type SiteLoader interface {
	GetSiteAggregateBySlug(ctx context.Context, slug string) (*domain.SiteAggregate, error)
	GetSiteBySlug(ctx context.Context, slug string) (*domain.Site, error)
	ResolveSlugRedirect(ctx context.Context, oldSlug string) (string, bool, error)
}

// MemberLoader is satisfied by *service.Members — checked alongside site
// ownership so an accepted teammate can reach a site's dashboard too, not
// just its owner.
type MemberLoader interface {
	IsAcceptedMember(ctx context.Context, siteID int, userID uuid.UUID) (bool, error)
}

// Ownership gates /dashboard/sites/{slug}/* routes so only a site's owner or
// an accepted team member can act on it. It loads the full site once and
// stashes it in the request context so handlers don't have to fetch it
// again.
type Ownership struct {
	sites   SiteLoader
	members MemberLoader
}

func NewOwnership(sites SiteLoader, members MemberLoader) *Ownership {
	return &Ownership{sites: sites, members: members}
}

// hasAccess reports whether userID may reach siteID's dashboard: either as
// the owner, or as an accepted team member.
func (o *Ownership) hasAccess(ctx context.Context, ownerUserID, userID uuid.UUID, siteID int) bool {
	if ownerUserID == userID {
		return true
	}
	ok, err := o.members.IsAcceptedMember(ctx, siteID, userID)
	return err == nil && ok
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
		if !o.hasAccess(r.Context(), site.OwnerUserID, UserID(r), site.ID) {
			http.NotFound(w, r)
			return
		}
		next(w, withSite(r, site))
	}
}

// RequireSiteOwnerLight is RequireSiteOwner's lightweight counterpart: it
// checks access from just the site's own row, without loading
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
		if !o.hasAccess(r.Context(), site.OwnerUserID, UserID(r), site.ID) {
			http.NotFound(w, r)
			return
		}
		next(w, withLightSite(r, site))
	}
}

// RequireOwnerRole re-checks that the caller is the site's actual owner —
// not just an accepted member — for routes that must stay owner-only (site
// deletion, billing, team management itself) even though RequireSiteOwner
// (Light) above now admits members too. Must run after one of those, so the
// site is already in the request context.
func (o *Ownership) RequireOwnerRole(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var ownerUserID uuid.UUID
		if site := LightSiteFromContext(r); site != nil {
			ownerUserID = site.OwnerUserID
		} else if site := SiteFromContext(r); site != nil {
			ownerUserID = site.OwnerUserID
		}
		if ownerUserID != UserID(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
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
