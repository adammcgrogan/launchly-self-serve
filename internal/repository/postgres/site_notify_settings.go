package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

func UpsertSiteNotifySettings(ctx context.Context, q querier, n *domain.SiteNotifySettings) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO site_notify_settings (site_id, mobile_number, sms_alerts_enabled)
		VALUES ($1, $2, $3)
		ON CONFLICT (site_id) DO UPDATE SET
			mobile_number = EXCLUDED.mobile_number, sms_alerts_enabled = EXCLUDED.sms_alerts_enabled
	`, n.SiteID, n.MobileNumber, n.SMSAlertsEnabled)
	return err
}

func GetSiteNotifySettings(ctx context.Context, q querier, siteID int) (*domain.SiteNotifySettings, error) {
	n := &domain.SiteNotifySettings{SiteID: siteID}
	err := q.QueryRowContext(ctx, `
		SELECT mobile_number, sms_alerts_enabled
		FROM site_notify_settings WHERE site_id = $1
	`, siteID).Scan(&n.MobileNumber, &n.SMSAlertsEnabled)
	if err == sql.ErrNoRows {
		return n, nil
	}
	if err != nil {
		return nil, err
	}
	return n, nil
}
