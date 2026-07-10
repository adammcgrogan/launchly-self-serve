package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteCertifications overwrites all trust badges/certifications for a site.
func ReplaceSiteCertifications(ctx context.Context, q querier, siteID int, certs []domain.Certification) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_certifications WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for i, c := range certs {
		if c.Label == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_certifications (site_id, label, sort_order) VALUES ($1, $2, $3)`,
			siteID, c.Label, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteCertifications(ctx context.Context, q querier, siteID int) ([]domain.Certification, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, site_id, label, sort_order FROM site_certifications WHERE site_id = $1 ORDER BY sort_order`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Certification
	for rows.Next() {
		var c domain.Certification
		if err := rows.Scan(&c.ID, &c.SiteID, &c.Label, &c.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
