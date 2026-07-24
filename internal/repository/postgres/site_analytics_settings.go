package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

func UpsertSiteAnalyticsSettings(ctx context.Context, q querier, a *domain.SiteAnalyticsSettings) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO site_analytics_settings (site_id, analytics_frequency)
		VALUES ($1, $2)
		ON CONFLICT (site_id) DO UPDATE SET
			analytics_frequency = EXCLUDED.analytics_frequency
	`, a.SiteID, a.AnalyticsFrequency)
	return err
}

func GetSiteAnalyticsSettings(ctx context.Context, q querier, siteID int) (*domain.SiteAnalyticsSettings, error) {
	a := &domain.SiteAnalyticsSettings{SiteID: siteID, AnalyticsFrequency: "off"}
	err := q.QueryRowContext(ctx, `
		SELECT analytics_frequency, analytics_last_sent_at, ga_measurement_id, meta_pixel_id
		FROM site_analytics_settings WHERE site_id = $1
	`, siteID).Scan(&a.AnalyticsFrequency, &a.AnalyticsLastSentAt, &a.GAMeasurementID, &a.MetaPixelID)
	if err == sql.ErrNoRows {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// UpsertSiteTrackingIDs saves a site's third-party tracking IDs without
// touching its analytics-report frequency, creating the settings row (with
// the default "off" frequency) if the site doesn't have one yet.
func UpsertSiteTrackingIDs(ctx context.Context, q querier, siteID int, gaMeasurementID, metaPixelID string) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO site_analytics_settings (site_id, analytics_frequency, ga_measurement_id, meta_pixel_id)
		VALUES ($1, 'off', $2, $3)
		ON CONFLICT (site_id) DO UPDATE SET
			ga_measurement_id = EXCLUDED.ga_measurement_id,
			meta_pixel_id = EXCLUDED.meta_pixel_id
	`, siteID, gaMeasurementID, metaPixelID)
	return err
}

func UpdateAnalyticsLastSent(ctx context.Context, q querier, siteID int) error {
	_, err := q.ExecContext(ctx, `UPDATE site_analytics_settings SET analytics_last_sent_at = now() WHERE site_id = $1`, siteID)
	return err
}

// DueAnalyticsDigest is a site whose monthly analytics digest is due, with
// the site/owner fields the cron sweep needs already joined in — see
// GetSitesDueForAnalytics.
type DueAnalyticsDigest struct {
	SiteID       int
	Slug         string
	BusinessName string
	Timezone     string
	NotifyEmail  string
}

// GetSitesDueForAnalytics returns sites whose monthly analytics digest is
// due, along with each site's resolved notification email (the account
// owner's login email, falling back to the site's public contact email —
// mirroring notifyEmail), joined in here so the cron sweep doesn't need a
// GetSiteByID/GetSiteContact/GetProfile per ID before building the digest
// (#218). The per-site stats computation and email send still happen one
// site at a time — those aren't lookups that can be batched away.
func GetSitesDueForAnalytics(ctx context.Context, q querier) ([]DueAnalyticsDigest, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT s.id, s.slug, s.business_name, s.timezone,
		       COALESCE(NULLIF(p.email, ''), c.email, '') AS notify_email
		FROM site_analytics_settings a
		JOIN sites s ON s.id = a.site_id
		LEFT JOIN profiles p ON p.id = s.owner_user_id
		LEFT JOIN site_contact c ON c.site_id = s.id
		WHERE a.analytics_frequency = 'monthly'
		  AND (a.analytics_last_sent_at IS NULL OR a.analytics_last_sent_at < now() - INTERVAL '30 days')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []DueAnalyticsDigest
	for rows.Next() {
		var d DueAnalyticsDigest
		if err := rows.Scan(&d.SiteID, &d.Slug, &d.BusinessName, &d.Timezone, &d.NotifyEmail); err != nil {
			return nil, err
		}
		due = append(due, d)
	}
	return due, rows.Err()
}
