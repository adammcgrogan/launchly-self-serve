package middleware

import (
	"context"
	"net/http"
	"strconv"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// SiteLoader is satisfied by *service.Sites — kept as an interface here so
// middleware doesn't need to import the whole service package's surface.
type SiteLoader interface {
	GetSiteAggregate(ctx context.Context, id int) (*domain.SiteAggregate, error)
}

// Ownership gates /dashboard/sites/{id}/* routes so a user can only act on
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
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		site, err := o.sites.GetSiteAggregate(r.Context(), id)
		if err != nil || site == nil {
			http.NotFound(w, r)
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
