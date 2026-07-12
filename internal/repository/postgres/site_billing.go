package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
)

// CreateSiteBilling creates the 1:1 billing row for a new site, starting a
// 14-day free trial with no card required.
func CreateSiteBilling(ctx context.Context, q querier, siteID int, plan domain.Plan) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO site_billing (site_id, plan, payment_status, trial_ends_at)
		VALUES ($1, $2, 'trialing', now() + INTERVAL '14 days')
	`, siteID, plan)
	return err
}

// OwnerHasProSite reports whether any of an account's sites is on the Pro
// plan, used to lift the per-account site cap (see
// service.Sites.canCreateSite) — plan is tracked per site, not per account.
func OwnerHasProSite(ctx context.Context, q querier, ownerID uuid.UUID) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM site_billing sb
			JOIN sites s ON s.id = sb.site_id
			WHERE s.owner_user_id = $1 AND sb.plan = 'pro'
		)
	`, ownerID).Scan(&exists)
	return exists, err
}

func scanSiteBilling(row *sql.Row, siteID int) (*domain.SiteBilling, error) {
	b := &domain.SiteBilling{SiteID: siteID}
	err := row.Scan(
		&b.Plan, &b.PaymentStatus, &b.StripeCustomerID, &b.StripeSessionID, &b.StripeSubscriptionID,
		&b.PaidAt, &b.TrialEndsAt, &b.TrialReminderSentAt, &b.TrialFinalReminderSentAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

const siteBillingColumns = `plan, payment_status, stripe_customer_id, stripe_session_id, stripe_subscription_id,
	paid_at, trial_ends_at, trial_reminder_sent_at, trial_final_reminder_sent_at`

func GetSiteBilling(ctx context.Context, q querier, siteID int) (*domain.SiteBilling, error) {
	return scanSiteBilling(q.QueryRowContext(ctx,
		`SELECT `+siteBillingColumns+` FROM site_billing WHERE site_id = $1`, siteID), siteID)
}

func GetSiteBillingBySessionID(ctx context.Context, q querier, sessionID string) (*domain.SiteBilling, error) {
	var siteID int
	b := &domain.SiteBilling{}
	err := q.QueryRowContext(ctx,
		`SELECT site_id, `+siteBillingColumns+` FROM site_billing WHERE stripe_session_id = $1`, sessionID,
	).Scan(&siteID, &b.Plan, &b.PaymentStatus, &b.StripeCustomerID, &b.StripeSessionID, &b.StripeSubscriptionID,
		&b.PaidAt, &b.TrialEndsAt, &b.TrialReminderSentAt, &b.TrialFinalReminderSentAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.SiteID = siteID
	return b, nil
}

func GetSiteBillingBySubscriptionID(ctx context.Context, q querier, subscriptionID string) (*domain.SiteBilling, error) {
	var siteID int
	b := &domain.SiteBilling{}
	err := q.QueryRowContext(ctx,
		`SELECT site_id, `+siteBillingColumns+` FROM site_billing WHERE stripe_subscription_id = $1`, subscriptionID,
	).Scan(&siteID, &b.Plan, &b.PaymentStatus, &b.StripeCustomerID, &b.StripeSessionID, &b.StripeSubscriptionID,
		&b.PaidAt, &b.TrialEndsAt, &b.TrialReminderSentAt, &b.TrialFinalReminderSentAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	b.SiteID = siteID
	return b, nil
}

// SetSitePending records that a Stripe Checkout session was created for a site's upgrade.
func SetSitePending(ctx context.Context, q querier, siteID int, plan domain.Plan, sessionID string) error {
	_, err := q.ExecContext(ctx,
		`UPDATE site_billing SET payment_status = 'pending', plan = $1, stripe_session_id = $2 WHERE site_id = $3`,
		plan, sessionID, siteID)
	return err
}

// SetSitePaid marks a site as paid by Stripe session ID. Returns (true, nil)
// if this was the first time (row updated), (false, nil) if already paid
// (idempotent webhook retry).
func SetSitePaid(ctx context.Context, q querier, sessionID, subscriptionID string) (bool, error) {
	now := time.Now().UTC()
	res, err := q.ExecContext(ctx, `
		UPDATE site_billing SET payment_status = 'paid', paid_at = $1, stripe_subscription_id = $2
		WHERE stripe_session_id = $3 AND payment_status != 'paid'
	`, now, subscriptionID, sessionID)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

func SetSiteCancelled(ctx context.Context, q querier, subscriptionID string) error {
	_, err := q.ExecContext(ctx, `UPDATE site_billing SET payment_status = 'cancelled' WHERE stripe_subscription_id = $1`, subscriptionID)
	return err
}

// GetSiteIDsDueForTrialReminder returns site IDs whose trial is ending soon
// and haven't yet received a reminder of the given kind ("first" = 3 days out,
// "final" = 1 day out).
func GetSiteIDsDueForTrialReminder(ctx context.Context, q querier, kind string) ([]int, error) {
	var query string
	switch kind {
	case "first":
		query = `SELECT site_id FROM site_billing
			WHERE trial_ends_at IS NOT NULL AND payment_status NOT IN ('paid', 'cancelled')
			  AND trial_ends_at <= now() + INTERVAL '3 days' AND trial_ends_at > now()
			  AND trial_reminder_sent_at IS NULL`
	case "final":
		query = `SELECT site_id FROM site_billing
			WHERE trial_ends_at IS NOT NULL AND payment_status NOT IN ('paid', 'cancelled')
			  AND trial_ends_at <= now() + INTERVAL '1 day' AND trial_ends_at > now()
			  AND trial_final_reminder_sent_at IS NULL`
	default:
		return nil, fmt.Errorf("unknown trial reminder kind: %s", kind)
	}
	rows, err := q.QueryContext(ctx, query)
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

// GetSiteIDsDueForTrialPause returns IDs of live sites whose trial ended
// before cutoff (i.e. trial_ends_at + grace period) with no paid
// subscription, so the trial cron can pause them.
func GetSiteIDsDueForTrialPause(ctx context.Context, q querier, cutoff time.Time) ([]int, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT sb.site_id FROM site_billing sb
		JOIN sites s ON s.id = sb.site_id
		WHERE sb.trial_ends_at IS NOT NULL
		  AND sb.trial_ends_at < $1
		  AND sb.payment_status != 'paid'
		  AND s.status = 'live'
	`, cutoff)
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

func MarkTrialReminderSent(ctx context.Context, q querier, siteID int, kind string) error {
	var col string
	switch kind {
	case "first":
		col = "trial_reminder_sent_at"
	case "final":
		col = "trial_final_reminder_sent_at"
	default:
		return fmt.Errorf("unknown trial reminder kind: %s", kind)
	}
	_, err := q.ExecContext(ctx, `UPDATE site_billing SET `+col+` = now() WHERE site_id = $1`, siteID)
	return err
}

// IsStripeEventProcessed reports whether a webhook event ID has already been
// recorded as processed. Checked before processing so retries of an
// already-fully-handled event are skipped; MarkStripeEventProcessed is only
// called after processing succeeds, so a transient failure mid-processing
// leaves the event unmarked and eligible for Stripe's automatic retry.
func IsStripeEventProcessed(ctx context.Context, q querier, eventID string) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM stripe_events WHERE event_id = $1)`, eventID,
	).Scan(&exists)
	return exists, err
}

// MarkStripeEventProcessed records a Stripe webhook event ID. Returns true if
// newly inserted (first delivery), false if already processed (retry/duplicate).
func MarkStripeEventProcessed(ctx context.Context, q querier, eventID string) (bool, error) {
	res, err := q.ExecContext(ctx,
		`INSERT INTO stripe_events (event_id) VALUES ($1) ON CONFLICT (event_id) DO NOTHING`, eventID)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}
