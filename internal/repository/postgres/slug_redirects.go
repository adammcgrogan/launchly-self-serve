package postgres

import (
	"context"
	"database/sql"
)

// CreateSlugRedirect records that oldSlug used to belong to siteID, so
// requests for it can be 301-redirected to the site's current slug.
func CreateSlugRedirect(ctx context.Context, q querier, oldSlug string, siteID int) error {
	_, err := q.ExecContext(ctx, `INSERT INTO slug_redirects (old_slug, site_id) VALUES ($1, $2)`, oldSlug, siteID)
	return err
}

// GetSlugRedirectSiteID looks up which site an old slug now redirects to.
// Returns 0, false if no redirect is recorded for it.
func GetSlugRedirectSiteID(ctx context.Context, q querier, oldSlug string) (int, bool, error) {
	var siteID int
	err := q.QueryRowContext(ctx, `SELECT site_id FROM slug_redirects WHERE old_slug = $1`, oldSlug).Scan(&siteID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return siteID, true, nil
}
