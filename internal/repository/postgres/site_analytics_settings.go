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
		SELECT analytics_frequency, analytics_last_sent_at
		FROM site_analytics_settings WHERE site_id = $1
	`, siteID).Scan(&a.AnalyticsFrequency, &a.AnalyticsLastSentAt)
	if err == sql.ErrNoRows {
		return a, nil
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

func UpdateAnalyticsLastSent(ctx context.Context, q querier, siteID int) error {
	_, err := q.ExecContext(ctx, `UPDATE site_analytics_settings SET analytics_last_sent_at = now() WHERE site_id = $1`, siteID)
	return err
}

// GetSiteIDsDueForAnalytics returns sites whose scheduled analytics digest is due.
func GetSiteIDsDueForAnalytics(ctx context.Context, q querier) ([]int, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT site_id FROM site_analytics_settings
		WHERE analytics_frequency != 'off'
		  AND (
		    analytics_last_sent_at IS NULL
		    OR (analytics_frequency = 'weekly' AND analytics_last_sent_at < now() - INTERVAL '7 days')
		    OR (analytics_frequency = 'monthly' AND analytics_last_sent_at < now() - INTERVAL '30 days')
		  )
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
