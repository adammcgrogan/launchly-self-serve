package service

import (
	"sync"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// Sites owns the create/edit/publish lifecycle of a site and assembling the
// full SiteAggregate from the many tables it spans.
//
// The rest of this package's site-related code is split by concern across
// sibling files: sites_validation.go (content/format validation),
// sites_lifecycle.go (create/publish/unpublish/delete/slug), sites_aggregate.go
// (GetSiteAggregate and lightweight getters), and sites_settings.go (the
// per-setting Update* methods).
type Sites struct {
	store   *postgres.Store
	billing *Billing
	cf      DomainRegistrar
	uploads *Uploads

	aggCacheMu sync.RWMutex
	aggCache   map[int]cachedAggregate
}

// siteAggregateCacheTTL bounds how stale a cached SiteAggregate can be. It's
// short enough that an unexpected miss on invalidation (e.g. a billing
// webhook updating SiteBilling outside the Sites service) self-heals within
// seconds, while still absorbing crawler/burst traffic on the public site
// render path (see #215).
const siteAggregateCacheTTL = 15 * time.Second

type cachedAggregate struct {
	agg     *domain.SiteAggregate
	expires time.Time
}

func NewSites(store *postgres.Store, billing *Billing, cf DomainRegistrar, uploads *Uploads) *Sites {
	return &Sites{store: store, billing: billing, cf: cf, uploads: uploads, aggCache: make(map[int]cachedAggregate)}
}

func (s *Sites) cachedAggregate(id int) (*domain.SiteAggregate, bool) {
	s.aggCacheMu.RLock()
	defer s.aggCacheMu.RUnlock()
	entry, ok := s.aggCache[id]
	if !ok || time.Now().After(entry.expires) {
		return nil, false
	}
	return entry.agg, true
}

func (s *Sites) cacheAggregate(id int, agg *domain.SiteAggregate) {
	s.aggCacheMu.Lock()
	defer s.aggCacheMu.Unlock()
	s.aggCache[id] = cachedAggregate{agg: agg, expires: time.Now().Add(siteAggregateCacheTTL)}
}

// invalidateAggregate drops a site's cached aggregate so the next render
// picks up an edit/publish immediately instead of waiting out the TTL.
func (s *Sites) invalidateAggregate(id int) {
	s.aggCacheMu.Lock()
	defer s.aggCacheMu.Unlock()
	delete(s.aggCache, id)
}
