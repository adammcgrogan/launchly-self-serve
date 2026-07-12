package postgres

import (
	"context"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteBusinessHours overwrites all opening-hours rows for a site.
func ReplaceSiteBusinessHours(ctx context.Context, q querier, siteID int, hours []domain.BusinessHours) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_business_hours WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for _, h := range hours {
		if !h.Closed && h.OpensAt == "" && h.ClosesAt == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_business_hours (site_id, weekday, opens_at, closes_at, closed) VALUES ($1, $2, $3, $4, $5)`,
			siteID, int(h.Weekday), h.OpensAt, h.ClosesAt, h.Closed,
		); err != nil {
			return err
		}
	}
	return nil
}

// GetSiteBusinessHours returns a site's opening-hours rows, Monday first.
func GetSiteBusinessHours(ctx context.Context, q querier, siteID int) ([]domain.BusinessHours, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, site_id, weekday, opens_at, closes_at, closed FROM site_business_hours WHERE site_id = $1 ORDER BY (weekday + 6) % 7`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BusinessHours
	for rows.Next() {
		var h domain.BusinessHours
		var weekday int
		if err := rows.Scan(&h.ID, &h.SiteID, &weekday, &h.OpensAt, &h.ClosesAt, &h.Closed); err != nil {
			return nil, err
		}
		h.Weekday = time.Weekday(weekday)
		out = append(out, h)
	}
	return out, rows.Err()
}
