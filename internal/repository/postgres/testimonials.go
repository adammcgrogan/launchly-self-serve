package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteTestimonials overwrites all testimonials for a site.
func ReplaceSiteTestimonials(ctx context.Context, q querier, siteID int, testimonials []domain.Testimonial) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_testimonials WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for i, t := range testimonials {
		if t.Quote == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_testimonials (site_id, author_name, author_role, quote, sort_order) VALUES ($1, $2, $3, $4, $5)`,
			siteID, t.AuthorName, t.AuthorRole, t.Quote, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteTestimonials(ctx context.Context, q querier, siteID int) ([]domain.Testimonial, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, site_id, author_name, author_role, quote, sort_order FROM site_testimonials WHERE site_id = $1 ORDER BY sort_order`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Testimonial
	for rows.Next() {
		var t domain.Testimonial
		if err := rows.Scan(&t.ID, &t.SiteID, &t.AuthorName, &t.AuthorRole, &t.Quote, &t.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
