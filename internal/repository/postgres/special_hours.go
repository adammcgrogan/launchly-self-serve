package postgres

import (
	"context"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteSpecialHours overwrites all date-scoped hours overrides for a site.
func ReplaceSiteSpecialHours(ctx context.Context, q querier, siteID int, hours []domain.SpecialHours) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_special_hours WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for _, h := range hours {
		if h.Date == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_special_hours (site_id, date, label, opens_at, closes_at, closed) VALUES ($1, $2, $3, $4, $5, $6)`,
			siteID, h.Date, h.Label, h.OpensAt, h.ClosesAt, h.Closed,
		); err != nil {
			return err
		}
	}
	return nil
}

// GetSiteSpecialHours returns a site's date-scoped hours overrides, soonest first.
func GetSiteSpecialHours(ctx context.Context, q querier, siteID int) ([]domain.SpecialHours, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, site_id, date, label, opens_at, closes_at, closed FROM site_special_hours WHERE site_id = $1 ORDER BY date`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.SpecialHours
	for rows.Next() {
		var h domain.SpecialHours
		var date time.Time
		if err := rows.Scan(&h.ID, &h.SiteID, &date, &h.Label, &h.OpensAt, &h.ClosesAt, &h.Closed); err != nil {
			return nil, err
		}
		h.Date = date.Format("2006-01-02")
		out = append(out, h)
	}
	return out, rows.Err()
}
