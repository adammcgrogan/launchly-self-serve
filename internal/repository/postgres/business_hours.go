package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteBusinessHours overwrites all opening-hours lines for a site.
func ReplaceSiteBusinessHours(ctx context.Context, q querier, siteID int, hours []domain.BusinessHours) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_business_hours WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for i, h := range hours {
		if h.Label == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_business_hours (site_id, label, hours_text, sort_order) VALUES ($1, $2, $3, $4)`,
			siteID, h.Label, h.HoursText, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteBusinessHours(ctx context.Context, q querier, siteID int) ([]domain.BusinessHours, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, site_id, label, hours_text, sort_order FROM site_business_hours WHERE site_id = $1 ORDER BY sort_order`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BusinessHours
	for rows.Next() {
		var h domain.BusinessHours
		if err := rows.Scan(&h.ID, &h.SiteID, &h.Label, &h.HoursText, &h.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
