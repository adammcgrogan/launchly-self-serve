package service

import (
	"testing"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

func TestSitesAggregateCache(t *testing.T) {
	s := &Sites{aggCache: make(map[int]cachedAggregate)}

	if _, ok := s.cachedAggregate(1); ok {
		t.Fatal("expected cache miss before anything is cached")
	}

	agg := &domain.SiteAggregate{Site: domain.Site{ID: 1}}
	s.cacheAggregate(1, agg)

	got, ok := s.cachedAggregate(1)
	if !ok || got != agg {
		t.Fatal("expected cache hit to return the cached aggregate")
	}

	s.invalidateAggregate(1)
	if _, ok := s.cachedAggregate(1); ok {
		t.Fatal("expected cache miss after invalidation")
	}

	s.cacheAggregate(1, agg)
	s.aggCache[1] = cachedAggregate{agg: agg, expires: time.Now().Add(-time.Second)}
	if _, ok := s.cachedAggregate(1); ok {
		t.Fatal("expected cache miss once entry has expired")
	}
}
