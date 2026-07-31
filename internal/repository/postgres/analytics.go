package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

func RecordPageView(ctx context.Context, q querier, pv *domain.PageView) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO page_views (site_id, path, referrer, visitor_hash) VALUES ($1, $2, $3, $4)`,
		pv.SiteID, pv.Path, pv.Referrer, pv.VisitorHash)
	return err
}

func RecordSiteEvent(ctx context.Context, q querier, e *domain.SiteEvent) error {
	_, err := q.ExecContext(ctx,
		`INSERT INTO site_events (site_id, kind, visitor_hash) VALUES ($1, $2, $3)`,
		e.SiteID, string(e.Kind), e.VisitorHash)
	return err
}

// PruneOldPageViews deletes page_views rows older than before, so the table
// doesn't grow forever now that the dashboard's analytics window is bounded.
func PruneOldPageViews(ctx context.Context, q querier, before time.Time) error {
	_, err := q.ExecContext(ctx, `DELETE FROM page_views WHERE created_at < $1`, before)
	return err
}

// PruneOldSiteEvents deletes site_events rows older than before, matching
// PruneOldPageViews' retention window.
func PruneOldSiteEvents(ctx context.Context, q querier, before time.Time) error {
	_, err := q.ExecContext(ctx, `DELETE FROM site_events WHERE created_at < $1`, before)
	return err
}

// GetSiteStats runs every aggregate (totals, top referrers, top pages, daily
// views, event-kind counts, and the prevSince..since comparison window) as a
// single round trip via CTEs + json_agg, rather than issuing each as its own
// query — this is called twice per dashboard render (7-day window and
// all-time), so each query added here would otherwise be two more
// full-table-aggregate round trips per page load.
//
// prevSince is the start of the period immediately preceding since, used for
// the period-over-period comparison fields (domain.SiteStats.PrevTotalViews
// et al). Pass the same value as since to skip the comparison — the
// (since, since] window is empty, so the Prev* fields come back zero.
func GetSiteStats(ctx context.Context, q querier, siteID int, since, prevSince time.Time, timezone string) (*domain.SiteStats, error) {
	// Postgres errors on an unrecognized zone name, so fall back the same
	// way domain.SiteAggregate.OpenNow does for an unset/invalid Timezone.
	if _, err := time.LoadLocation(timezone); err != nil {
		timezone = "UTC"
	}
	stats := &domain.SiteStats{
		PeriodDays: int(time.Since(since).Hours()/24) + 1,
	}

	var referrersJSON, pagesJSON, daysJSON, eventsJSON []byte
	err := q.QueryRowContext(ctx, `
		WITH views AS (
			SELECT COUNT(*) AS total, COUNT(DISTINCT NULLIF(visitor_hash, '')) AS uniques
			FROM page_views WHERE site_id = $1 AND created_at > $2
		), prev_views AS (
			SELECT COUNT(*) AS total, COUNT(DISTINCT NULLIF(visitor_hash, '')) AS uniques
			FROM page_views WHERE site_id = $1 AND created_at > $4 AND created_at <= $2
		), referrers AS (
			SELECT referrer, COUNT(*) AS count FROM page_views
			WHERE site_id = $1 AND created_at > $2 AND referrer != ''
			GROUP BY referrer ORDER BY count DESC LIMIT 5
		), pages AS (
			SELECT path, COUNT(*) AS count FROM page_views
			WHERE site_id = $1 AND created_at > $2
			GROUP BY path ORDER BY count DESC LIMIT 5
		), days AS (
			SELECT (date_trunc('day', created_at AT TIME ZONE $3) AT TIME ZONE $3) AS day, COUNT(*) AS count
			FROM page_views WHERE site_id = $1 AND created_at > $2
			GROUP BY day ORDER BY day
		), events AS (
			SELECT kind, COUNT(*) AS count FROM site_events
			WHERE site_id = $1 AND created_at > $2
			GROUP BY kind
		), prev_events AS (
			SELECT COUNT(*) AS count FROM site_events
			WHERE site_id = $1 AND kind = 'lead' AND created_at > $4 AND created_at <= $2
		)
		SELECT
			(SELECT total FROM views), (SELECT uniques FROM views),
			(SELECT total FROM prev_views), (SELECT uniques FROM prev_views),
			COALESCE((SELECT json_agg(referrers) FROM referrers), '[]'),
			COALESCE((SELECT json_agg(pages) FROM pages), '[]'),
			COALESCE((SELECT json_agg(days) FROM days), '[]'),
			COALESCE((SELECT json_agg(events) FROM events), '[]'),
			(SELECT count FROM prev_events)
	`, siteID, since, timezone, prevSince).Scan(
		&stats.TotalViews, &stats.UniqueVisitors,
		&stats.PrevTotalViews, &stats.PrevUniqueVisitors,
		&referrersJSON, &pagesJSON, &daysJSON, &eventsJSON,
		&stats.PrevLeads,
	)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(referrersJSON, &stats.TopReferrers); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(pagesJSON, &stats.TopPages); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(daysJSON, &stats.ViewsByDay); err != nil {
		return nil, err
	}
	var eventCounts []struct {
		Kind  string `json:"kind"`
		Count int    `json:"count"`
	}
	if err := json.Unmarshal(eventsJSON, &eventCounts); err != nil {
		return nil, err
	}
	for _, ec := range eventCounts {
		switch domain.EventKind(ec.Kind) {
		case domain.EventKindCall:
			stats.CallTaps = ec.Count
		case domain.EventKindWhatsApp:
			stats.WhatsAppTaps = ec.Count
		case domain.EventKindDirections:
			stats.DirectionsClicks = ec.Count
		case domain.EventKindDownload:
			stats.DocumentDownloads = ec.Count
		case domain.EventKindLead:
			stats.Leads = ec.Count
		}
	}
	return stats, nil
}
