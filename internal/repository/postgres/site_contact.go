package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// UpsertSiteContact creates or updates a site's 1:1 contact row.
func UpsertSiteContact(ctx context.Context, q querier, c *domain.SiteContact) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO site_contact (site_id, phone, email, address, location, map_url, map_embed_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (site_id) DO UPDATE SET
			phone = EXCLUDED.phone, email = EXCLUDED.email, address = EXCLUDED.address,
			location = EXCLUDED.location, map_url = EXCLUDED.map_url, map_embed_url = EXCLUDED.map_embed_url
	`, c.SiteID, c.Phone, c.Email, c.Address, c.Location, c.MapURL, c.MapEmbedURL)
	return err
}

// GetSiteContact returns a zero-value SiteContact (not an error) if no row exists yet.
func GetSiteContact(ctx context.Context, q querier, siteID int) (*domain.SiteContact, error) {
	c := &domain.SiteContact{SiteID: siteID}
	err := q.QueryRowContext(ctx, `
		SELECT phone, email, address, location, map_url, map_embed_url
		FROM site_contact WHERE site_id = $1
	`, siteID).Scan(&c.Phone, &c.Email, &c.Address, &c.Location, &c.MapURL, &c.MapEmbedURL)
	if err == sql.ErrNoRows {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}
