package postgres

import (
	"context"
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

func GetSiteStats(ctx context.Context, q querier, siteID int, since time.Time, timezone string) (*domain.SiteStats, error) {
	// Postgres errors on an unrecognized zone name, so fall back the same
	// way domain.SiteAggregate.OpenNow does for an unset/invalid Timezone.
	if _, err := time.LoadLocation(timezone); err != nil {
		timezone = "UTC"
	}
	stats := &domain.SiteStats{
		PeriodDays: int(time.Since(since).Hours()/24) + 1,
	}

	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM page_views WHERE site_id = $1 AND created_at > $2`, siteID, since,
	).Scan(&stats.TotalViews); err != nil {
		return nil, err
	}

	if err := q.QueryRowContext(ctx,
		`SELECT COUNT(DISTINCT visitor_hash) FROM page_views WHERE site_id = $1 AND created_at > $2 AND visitor_hash != ''`, siteID, since,
	).Scan(&stats.UniqueVisitors); err != nil {
		return nil, err
	}

	refRows, err := q.QueryContext(ctx, `
		SELECT referrer, COUNT(*) AS cnt FROM page_views
		WHERE site_id = $1 AND created_at > $2 AND referrer != ''
		GROUP BY referrer ORDER BY cnt DESC LIMIT 5
	`, siteID, since)
	if err != nil {
		return nil, err
	}
	defer refRows.Close()
	for refRows.Next() {
		var rc domain.ReferrerCount
		if err := refRows.Scan(&rc.Referrer, &rc.Count); err != nil {
			return nil, err
		}
		stats.TopReferrers = append(stats.TopReferrers, rc)
	}
	if err := refRows.Err(); err != nil {
		return nil, err
	}

	dayRows, err := q.QueryContext(ctx, `
		SELECT date_trunc('day', created_at AT TIME ZONE $3) AS day, COUNT(*) AS cnt
		FROM page_views WHERE site_id = $1 AND created_at > $2
		GROUP BY day ORDER BY day
	`, siteID, since, timezone)
	if err != nil {
		return nil, err
	}
	defer dayRows.Close()
	for dayRows.Next() {
		var dc domain.DayCount
		if err := dayRows.Scan(&dc.Day, &dc.Count); err != nil {
			return nil, err
		}
		stats.ViewsByDay = append(stats.ViewsByDay, dc)
	}
	if err := dayRows.Err(); err != nil {
		return nil, err
	}

	eventRows, err := q.QueryContext(ctx, `
		SELECT kind, COUNT(*) FROM site_events
		WHERE site_id = $1 AND created_at > $2
		GROUP BY kind
	`, siteID, since)
	if err != nil {
		return nil, err
	}
	defer eventRows.Close()
	for eventRows.Next() {
		var kind string
		var count int
		if err := eventRows.Scan(&kind, &count); err != nil {
			return nil, err
		}
		switch domain.EventKind(kind) {
		case domain.EventKindCall:
			stats.CallTaps = count
		case domain.EventKindWhatsApp:
			stats.WhatsAppTaps = count
		case domain.EventKindDirections:
			stats.DirectionsClicks = count
		case domain.EventKindLead:
			stats.Leads = count
		}
	}
	return stats, eventRows.Err()
}
