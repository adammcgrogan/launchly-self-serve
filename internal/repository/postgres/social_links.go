package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteSocialLinks overwrites all social links for a site with the
// given set — the editor form always submits the full list, so a delete +
// bulk insert is simpler and safer than diffing.
func ReplaceSiteSocialLinks(ctx context.Context, q querier, siteID int, links []domain.SocialLink) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_social_links WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for _, l := range links {
		if l.URL == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_social_links (site_id, platform, url) VALUES ($1, $2, $3)`,
			siteID, l.Platform, l.URL,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteSocialLinks(ctx context.Context, q querier, siteID int) ([]domain.SocialLink, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, site_id, platform, url FROM site_social_links WHERE site_id = $1 ORDER BY platform`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []domain.SocialLink
	for rows.Next() {
		var l domain.SocialLink
		if err := rows.Scan(&l.ID, &l.SiteID, &l.Platform, &l.URL); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}
