package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteGalleryImages overwrites all gallery images for a site.
func ReplaceSiteGalleryImages(ctx context.Context, q querier, siteID int, images []domain.GalleryImage) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_gallery_images WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for i, img := range images {
		if img.URL == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_gallery_images (site_id, url, alt_text, sort_order) VALUES ($1, $2, $3, $4)`,
			siteID, img.URL, img.AltText, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteGalleryImages(ctx context.Context, q querier, siteID int) ([]domain.GalleryImage, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, site_id, url, alt_text, sort_order FROM site_gallery_images WHERE site_id = $1 ORDER BY sort_order`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.GalleryImage
	for rows.Next() {
		var g domain.GalleryImage
		if err := rows.Scan(&g.ID, &g.SiteID, &g.URL, &g.AltText, &g.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
