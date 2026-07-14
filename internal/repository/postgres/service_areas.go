package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteServiceAreas overwrites all served-area rows for a site.
func ReplaceSiteServiceAreas(ctx context.Context, q querier, siteID int, areas []domain.ServiceArea) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_service_areas WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for i, a := range areas {
		if a.Area == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_service_areas (site_id, area, sort_order) VALUES ($1, $2, $3)`,
			siteID, a.Area, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteServiceAreas(ctx context.Context, q querier, siteID int) ([]domain.ServiceArea, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, site_id, area, sort_order FROM site_service_areas WHERE site_id = $1 ORDER BY sort_order`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ServiceArea
	for rows.Next() {
		var a domain.ServiceArea
		if err := rows.Scan(&a.ID, &a.SiteID, &a.Area, &a.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
