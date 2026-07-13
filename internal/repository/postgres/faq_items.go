package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteFAQItems overwrites all FAQ items for a site.
func ReplaceSiteFAQItems(ctx context.Context, q querier, siteID int, items []domain.FAQItem) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_faq_items WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for i, f := range items {
		if f.Question == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_faq_items (site_id, question, answer, sort_order) VALUES ($1, $2, $3, $4)`,
			siteID, f.Question, f.Answer, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteFAQItems(ctx context.Context, q querier, siteID int) ([]domain.FAQItem, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, site_id, question, answer, sort_order FROM site_faq_items WHERE site_id = $1 ORDER BY sort_order`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.FAQItem
	for rows.Next() {
		var f domain.FAQItem
		if err := rows.Scan(&f.ID, &f.SiteID, &f.Question, &f.Answer, &f.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
