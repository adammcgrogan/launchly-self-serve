// Package middleware holds HTTP middleware: authentication (Supabase JWTs
// for customers, a separate shared-password session for the superadmin),
// site-ownership checks, CSRF, rate limiting, and flash messages.
package middleware

import (
	"context"
	"net/http"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
)

type ctxKey int

const (
	userIDCtxKey ctxKey = iota
	siteCtxKey
	siteLightCtxKey
	requestIDCtxKey
)

// UserID returns the authenticated user's ID, or the zero UUID if RequireUser
// hasn't run on this request.
func UserID(r *http.Request) uuid.UUID {
	id, _ := r.Context().Value(userIDCtxKey).(uuid.UUID)
	return id
}

func withUserID(r *http.Request, id uuid.UUID) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), userIDCtxKey, id))
}

// SiteFromContext returns the site loaded by RequireSiteOwner, avoiding a
// second fetch in the handler.
func SiteFromContext(r *http.Request) *domain.SiteAggregate {
	s, _ := r.Context().Value(siteCtxKey).(*domain.SiteAggregate)
	return s
}

func withSite(r *http.Request, site *domain.SiteAggregate) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), siteCtxKey, site))
}

// LightSiteFromContext returns the site loaded by RequireSiteOwnerLight —
// just the site's own row, not the full aggregate — for handlers that only
// need core fields like ID/Slug and don't want to pay for the aggregate's
// full fan-out of queries.
func LightSiteFromContext(r *http.Request) *domain.Site {
	s, _ := r.Context().Value(siteLightCtxKey).(*domain.Site)
	return s
}

func withLightSite(r *http.Request, site *domain.Site) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), siteLightCtxKey, site))
}
