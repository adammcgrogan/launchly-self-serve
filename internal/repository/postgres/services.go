package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteServices overwrites all service line items for a site.
func ReplaceSiteServices(ctx context.Context, q querier, siteID int, services []domain.Service) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_services WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for i, svc := range services {
		if svc.Label == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_services (site_id, label, sort_order) VALUES ($1, $2, $3)`,
			siteID, svc.Label, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteServices(ctx context.Context, q querier, siteID int) ([]domain.Service, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, site_id, label, sort_order FROM site_services WHERE site_id = $1 ORDER BY sort_order`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Service
	for rows.Next() {
		var s domain.Service
		if err := rows.Scan(&s.ID, &s.SiteID, &s.Label, &s.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
