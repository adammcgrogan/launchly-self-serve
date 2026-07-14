package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

func UpsertSiteReviews(ctx context.Context, q querier, r *domain.SiteReviews) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO site_reviews (site_id, rating, review_count, review_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (site_id) DO UPDATE SET
			rating = EXCLUDED.rating,
			review_count = EXCLUDED.review_count,
			review_url = EXCLUDED.review_url
	`, r.SiteID, r.Rating, r.ReviewCount, r.ReviewURL)
	return err
}

func GetSiteReviews(ctx context.Context, q querier, siteID int) (*domain.SiteReviews, error) {
	r := &domain.SiteReviews{SiteID: siteID}
	err := q.QueryRowContext(ctx, `
		SELECT rating, review_count, review_url FROM site_reviews WHERE site_id = $1
	`, siteID).Scan(&r.Rating, &r.ReviewCount, &r.ReviewURL)
	if err == sql.ErrNoRows {
		return r, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}
