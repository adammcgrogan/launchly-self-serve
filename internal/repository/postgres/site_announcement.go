package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// UpsertSiteAnnouncement creates or updates a site's 1:1 announcement row.
func UpsertSiteAnnouncement(ctx context.Context, q querier, a *domain.SiteAnnouncement) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO site_announcements (site_id, text, expires_at, tone, link_url, link_label)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (site_id) DO UPDATE SET
			text = EXCLUDED.text,
			expires_at = EXCLUDED.expires_at,
			tone = EXCLUDED.tone,
			link_url = EXCLUDED.link_url,
			link_label = EXCLUDED.link_label
	`, a.SiteID, a.Text, a.ExpiresAt, a.Tone, a.LinkURL, a.LinkLabel)
	return err
}

// GetSiteAnnouncement returns a zero-value SiteAnnouncement (not an error) if no row exists yet.
func GetSiteAnnouncement(ctx context.Context, q querier, siteID int) (*domain.SiteAnnouncement, error) {
	a := &domain.SiteAnnouncement{SiteID: siteID}
	err := q.QueryRowContext(ctx, `
		SELECT text, expires_at, tone, link_url, link_label FROM site_announcements WHERE site_id = $1
	`, siteID).Scan(&a.Text, &a.ExpiresAt, &a.Tone, &a.LinkURL, &a.LinkLabel)
	if err == sql.ErrNoRows {
		a.Tone = domain.AnnouncementInfo
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}
