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

func GetSiteStats(ctx context.Context, q querier, siteID int, since time.Time) (*domain.SiteStats, error) {
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
		SELECT date_trunc('day', created_at AT TIME ZONE 'UTC') AS day, COUNT(*) AS cnt
		FROM page_views WHERE site_id = $1 AND created_at > $2
		GROUP BY day ORDER BY day
	`, siteID, since)
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
	return stats, dayRows.Err()
}
