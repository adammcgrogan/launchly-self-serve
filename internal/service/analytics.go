package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// Analytics records page views and computes site stats. Visitor IPs are
// never stored raw — only a salted hash, just enough to approximate unique
// visitor counts.
type Analytics struct {
	store *postgres.Store
	salt  string
}

func NewAnalytics(store *postgres.Store, salt string) *Analytics {
	return &Analytics{store: store, salt: salt}
}

func (a *Analytics) hashVisitor(ip string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(a.salt + ip))
	return hex.EncodeToString(sum[:])
}

func (a *Analytics) RecordPageView(ctx context.Context, siteID int, path, referrer, ip string) error {
	pv := &domain.PageView{SiteID: siteID, Path: path, Referrer: referrer, VisitorHash: a.hashVisitor(ip)}
	return postgres.RecordPageView(ctx, a.store.DB(), pv)
}

// RecordEvent records a conversion (call tap, WhatsApp tap, directions
// click) fired via the sendBeacon endpoint. Lead conversions are recorded
// separately, server-side, by Leads.SubmitLead.
func (a *Analytics) RecordEvent(ctx context.Context, siteID int, kind domain.EventKind, ip string) error {
	e := &domain.SiteEvent{SiteID: siteID, Kind: kind, VisitorHash: a.hashVisitor(ip)}
	return postgres.RecordSiteEvent(ctx, a.store.DB(), e)
}

func (a *Analytics) GetSiteStats(ctx context.Context, siteID int, since time.Time) (*domain.SiteStats, error) {
	return postgres.GetSiteStats(ctx, a.store.DB(), siteID, since)
}
