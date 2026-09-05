package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
)

// EnsureAccountBilling creates an account's billing row on first site
// creation, starting a 7-day free trial with no card required. It's a no-op
// for an account that already has one — billing is per-account (#338), so
// the second and later sites an account creates must inherit the existing
// plan rather than starting a trial of their own.
//
// Trials are Starter-only; Pro is reached only via CreateUpgradeCheckout,
// which requires a completed Stripe checkout before the plan takes effect
// (see SetAccountPending/SetAccountPaid).
func EnsureAccountBilling(ctx context.Context, q querier, ownerID uuid.UUID, plan domain.Plan) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO account_billing (owner_user_id, plan, payment_status, trial_ends_at)
		VALUES ($1, $2, 'trialing', now() + INTERVAL '7 days')
		ON CONFLICT (owner_user_id) DO NOTHING
	`, ownerID, plan)
	return err
}

// EnsureDemoAccountBilling marks a seeded demo account as already paid Pro
// with no trial — demo sites are permanent showcases, not subject to the
// trial-expiry pause cron (which only acts on payment_status = 'trialing').
func EnsureDemoAccountBilling(ctx context.Context, q querier, ownerID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := q.ExecContext(ctx, `
		INSERT INTO account_billing (owner_user_id, plan, payment_status, paid_at)
		VALUES ($1, 'pro', 'paid', $2)
		ON CONFLICT (owner_user_id) DO UPDATE SET
			plan = 'pro', payment_status = 'paid', paid_at = EXCLUDED.paid_at
	`, ownerID, now)
	return err
}

// OwnerHasProPlan reports whether an account has active, paid Pro access,
// used to lift the per-account site cap (see service.Sites.canCreateSite).
// payment_status = 'paid' is required alongside plan = 'pro' so an abandoned
// or cancelled checkout (plan flips to 'pro' before payment completes)
// doesn't lift the cap for free.
func OwnerHasProPlan(ctx context.Context, q querier, ownerID uuid.UUID) (bool, error) {
	var exists bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM account_billing
			WHERE owner_user_id = $1 AND plan = 'pro' AND payment_status = 'paid'
		)
	`, ownerID).Scan(&exists)
	return exists, err
}

// scanAccountBilling scans an owner_user_id, accountBillingColumns row —
// every caller selects owner_user_id first (even GetAccountBilling, which
// already knows it from its own ownerID argument) so this one scan body
// covers every lookup.
func scanAccountBilling(row scanner) (*domain.AccountBilling, error) {
	b := &domain.AccountBilling{}
	err := row.Scan(
		&b.OwnerUserID, &b.Plan, &b.PaymentStatus, &b.StripeCustomerID, &b.StripeSessionID, &b.StripeSubscriptionID,
		&b.PaidAt, &b.TrialEndsAt, &b.TrialReminderSentAt, &b.TrialFinalReminderSentAt,
		&b.PaymentFailedAt, &b.DunningReminder1SentAt, &b.DunningReminder2SentAt, &b.DunningFinalWarningSentAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return b, nil
}

const accountBillingColumns = `plan, payment_status, stripe_customer_id, stripe_session_id, stripe_subscription_id,
	paid_at, trial_ends_at, trial_reminder_sent_at, trial_final_reminder_sent_at,
	payment_failed_at, dunning_reminder_1_sent_at, dunning_reminder_2_sent_at, dunning_final_warning_sent_at`

func GetAccountBilling(ctx context.Context, q querier, ownerID uuid.UUID) (*domain.AccountBilling, error) {
	return scanAccountBilling(q.QueryRowContext(ctx,
		`SELECT owner_user_id, `+accountBillingColumns+` FROM account_billing WHERE owner_user_id = $1`, ownerID))
}

// GetAccountBillingBySiteID resolves a site's billing through its owner, so
// the per-site Pro gates (custom domain, SMS alerts, the Launchly badge) can
// keep asking "is this site Pro?" without every caller having to carry an
// owner ID around.
func GetAccountBillingBySiteID(ctx context.Context, q querier, siteID int) (*domain.AccountBilling, error) {
	return scanAccountBilling(q.QueryRowContext(ctx,
		`SELECT ab.owner_user_id, `+accountBillingColumns+`
		 FROM account_billing ab
		 JOIN sites s ON s.owner_user_id = ab.owner_user_id
		 WHERE s.id = $1`, siteID))
}

func GetAccountBillingBySessionID(ctx context.Context, q querier, sessionID string) (*domain.AccountBilling, error) {
	return scanAccountBilling(q.QueryRowContext(ctx,
		`SELECT owner_user_id, `+accountBillingColumns+` FROM account_billing WHERE stripe_session_id = $1`, sessionID))
}

func GetAccountBillingBySubscriptionID(ctx context.Context, q querier, subscriptionID string) (*domain.AccountBilling, error) {
	return scanAccountBilling(q.QueryRowContext(ctx,
		`SELECT owner_user_id, `+accountBillingColumns+` FROM account_billing WHERE stripe_subscription_id = $1`, subscriptionID))
}

// SetAccountPending records that a Stripe Checkout session was created for
// an account's upgrade. It refuses to touch a row that's already 'paid' — an
// abandoned checkout must not clobber a settled, paying customer's billing
// state (see CreateUpgradeCheckout, which routes already-paid accounts
// through ChangeSubscriptionPlan instead of a new checkout session in the
// first place, but this guard holds regardless of caller).
func SetAccountPending(ctx context.Context, q querier, ownerID uuid.UUID, plan domain.Plan, sessionID string) error {
	_, err := q.ExecContext(ctx,
		`UPDATE account_billing SET payment_status = 'pending', plan = $1, stripe_session_id = $2
		 WHERE owner_user_id = $3 AND payment_status != 'paid'`,
		plan, sessionID, ownerID)
	return err
}

// SetAccountPaid marks an account as paid by Stripe session ID. Returns
// (true, nil) if this was the first time (row updated), (false, nil) if
// already paid (idempotent webhook retry). Also clears any in-flight dunning
// state, so an account that was mid-checkout while past due (e.g.
// re-subscribing after cancellation) doesn't carry stale payment-failure
// timestamps forward, and records the Stripe customer the billing portal is
// later opened against (only when Stripe sent one, so a retry missing it
// can't blank it).
func SetAccountPaid(ctx context.Context, q querier, sessionID, subscriptionID, customerID string) (bool, error) {
	now := time.Now().UTC()
	res, err := q.ExecContext(ctx, `
		UPDATE account_billing SET payment_status = 'paid', paid_at = $1, stripe_subscription_id = $2,
			stripe_customer_id = COALESCE(NULLIF($4, ''), stripe_customer_id),
			payment_failed_at = NULL, dunning_reminder_1_sent_at = NULL,
			dunning_reminder_2_sent_at = NULL, dunning_final_warning_sent_at = NULL
		WHERE stripe_session_id = $3 AND payment_status != 'paid'
	`, now, subscriptionID, sessionID, customerID)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// SetStripeCustomerID records the Stripe customer for an account. Called
// both from the webhook path (which learns the ID from the event) and from
// the billing-portal backfill for accounts that subscribed before the ID was
// persisted — see service.Billing.stripeCustomerID.
func SetStripeCustomerID(ctx context.Context, q querier, ownerID uuid.UUID, customerID string) error {
	_, err := q.ExecContext(ctx,
		`UPDATE account_billing SET stripe_customer_id = $1 WHERE owner_user_id = $2`, customerID, ownerID)
	return err
}

// SetAccountPaymentFailed transitions an account into the past-due dunning
// sequence the first time a payment fails, recording payment_failed_at as
// the anchor for the dunning cron's escalating reminders. Guarded to
// payment_status != 'past_due' so repeat invoice.payment_failed deliveries
// for the same underlying failure (Stripe retries several times before
// giving up) don't keep resetting the clock and delaying the sequence
// indefinitely. Returns true if this call caused the transition.
func SetAccountPaymentFailed(ctx context.Context, q querier, subscriptionID string) (bool, error) {
	res, err := q.ExecContext(ctx, `
		UPDATE account_billing SET payment_status = 'past_due', payment_failed_at = now()
		WHERE stripe_subscription_id = $1 AND payment_status NOT IN ('past_due', 'cancelled')
	`, subscriptionID)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// SetAccountPaymentRecovered moves an account back to 'paid' once Stripe
// confirms an invoice succeeded, clearing the dunning sequence. Only acts on
// accounts currently 'past_due' — a routine invoice.payment_succeeded for an
// account that's already 'paid' is not a recovery and shouldn't reset
// anything. Returns true if this call performed the recovery, so the caller
// knows whether to send a confirmation email.
func SetAccountPaymentRecovered(ctx context.Context, q querier, subscriptionID string) (bool, error) {
	res, err := q.ExecContext(ctx, `
		UPDATE account_billing SET payment_status = 'paid', payment_failed_at = NULL,
			dunning_reminder_1_sent_at = NULL, dunning_reminder_2_sent_at = NULL,
			dunning_final_warning_sent_at = NULL
		WHERE stripe_subscription_id = $1 AND payment_status = 'past_due'
	`, subscriptionID)
	if err != nil {
		return false, err
	}
	rows, _ := res.RowsAffected()
	return rows > 0, nil
}

// SetAccountPlan updates the plan for an account whose existing Stripe
// subscription was changed in place (see payment.Client.ChangeSubscriptionPlan)
// — there's no new checkout session or webhook involved, so the plan is
// recorded here directly once Stripe confirms the swap.
func SetAccountPlan(ctx context.Context, q querier, ownerID uuid.UUID, plan domain.Plan) error {
	_, err := q.ExecContext(ctx, `UPDATE account_billing SET plan = $1 WHERE owner_user_id = $2`, plan, ownerID)
	return err
}

// SetAccountCancelled marks an account's subscription cancelled by Stripe
// subscription ID.
func SetAccountCancelled(ctx context.Context, q querier, subscriptionID string) error {
	_, err := q.ExecContext(ctx,
		`UPDATE account_billing SET payment_status = 'cancelled' WHERE stripe_subscription_id = $1`, subscriptionID)
	return err
}

// DueTrialReminder is an account due for a trial reminder email, with every
// field the cron sweep needs (the account, its representative site, and the
// resolved notification email) already joined in — see
// GetAccountsDueForTrialReminder.
type DueTrialReminder struct {
	OwnerUserID  uuid.UUID
	SiteID       int
	Slug         string
	BusinessName string
	TrialEndsAt  time.Time
	Timezone     string
	NotifyEmail  string
}

// GetAccountsDueForTrialReminder returns accounts whose trial is ending soon
// and haven't yet received a reminder of the given kind, along with the
// account's resolved notification email (the owner's login email, falling
// back to the site's public contact email — mirroring notifyEmail) computed
// in the same query. This joins in everything the cron sweep's per-account
// loop needs so it doesn't have to follow up with a
// GetSiteByID/GetSiteContact/GetProfile per row (#218).
//
// The kinds, on a 7-day trial, in the order they land:
//
//	"report_early" — day 3, 4 days out: the first value report
//	"first"        — day 4, 3 days out: first upgrade nudge
//	"report"       — day 5, 2 days out: the fuller value report
//	"final"        — day 6, 1 day out:  last upgrade nudge
//
// The two report kinds are what stop every trial email being about billing
// (#330) — each lands the day before an upgrade ask, so the ask follows
// evidence rather than replacing it.
//
// Reports are only sent about live sites: a draft site has no public traffic
// to report on, so a "here's how you did" email about an unpublished site
// would be noise. The upgrade nudges still go to accounts whose site is a
// draft, since those are about the trial expiring rather than about traffic.
//
// A trialing account is capped at one site (see canCreateSite), so the
// DISTINCT ON picking the account's oldest site is exact rather than a
// heuristic — there is only ever one to pick.
func GetAccountsDueForTrialReminder(ctx context.Context, q querier, kind string) ([]DueTrialReminder, error) {
	var cond string
	switch kind {
	case "first":
		cond = `ab.trial_ends_at <= now() + INTERVAL '3 days' AND ab.trial_ends_at > now()
			  AND ab.trial_reminder_sent_at IS NULL`
	case "final":
		cond = `ab.trial_ends_at <= now() + INTERVAL '1 day' AND ab.trial_ends_at > now()
			  AND ab.trial_final_reminder_sent_at IS NULL`
	case "report_early":
		cond = `ab.trial_ends_at <= now() + INTERVAL '4 days' AND ab.trial_ends_at > now()
			  AND ab.trial_early_report_sent_at IS NULL AND s.status = 'live'`
	case "report":
		cond = `ab.trial_ends_at <= now() + INTERVAL '2 days' AND ab.trial_ends_at > now()
			  AND ab.trial_report_sent_at IS NULL AND s.status = 'live'`
	default:
		return nil, fmt.Errorf("unknown trial reminder kind: %s", kind)
	}
	rows, err := q.QueryContext(ctx, `
		SELECT DISTINCT ON (ab.owner_user_id)
		       ab.owner_user_id, s.id, s.slug, s.business_name, ab.trial_ends_at, s.timezone,
		       COALESCE(NULLIF(p.email, ''), c.email, '') AS notify_email
		FROM account_billing ab
		JOIN sites s ON s.owner_user_id = ab.owner_user_id
		LEFT JOIN profiles p ON p.id = ab.owner_user_id
		LEFT JOIN site_contact c ON c.site_id = s.id
		WHERE ab.trial_ends_at IS NOT NULL AND ab.payment_status NOT IN ('paid', 'cancelled')
		  AND `+cond+`
		ORDER BY ab.owner_user_id, s.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []DueTrialReminder
	for rows.Next() {
		var d DueTrialReminder
		if err := rows.Scan(&d.OwnerUserID, &d.SiteID, &d.Slug, &d.BusinessName, &d.TrialEndsAt, &d.Timezone, &d.NotifyEmail); err != nil {
			return nil, err
		}
		due = append(due, d)
	}
	return due, rows.Err()
}

// DueTrialPause is a live site whose owning account's trial has ended with no
// paid subscription, with every field the cron sweep needs already joined in
// — see GetSitesDueForTrialPause. One expired account can yield several rows,
// one per live site, since the whole account goes dark together (#338).
type DueTrialPause struct {
	OwnerUserID  uuid.UUID
	SiteID       int
	Slug         string
	BusinessName string
	NotifyEmail  string
}

// GetSitesDueForTrialPause returns live sites whose account's trial ended
// before cutoff (i.e. trial_ends_at + grace period) with no paid
// subscription, along with each site's resolved notification email, joined
// in here so the trial cron doesn't need a
// GetSiteByID/GetSiteContact/GetProfile per ID after pausing (#218).
func GetSitesDueForTrialPause(ctx context.Context, q querier, cutoff time.Time) ([]DueTrialPause, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT ab.owner_user_id, s.id, s.slug, s.business_name,
		       COALESCE(NULLIF(p.email, ''), c.email, '') AS notify_email
		FROM account_billing ab
		JOIN sites s ON s.owner_user_id = ab.owner_user_id
		LEFT JOIN profiles p ON p.id = ab.owner_user_id
		LEFT JOIN site_contact c ON c.site_id = s.id
		WHERE ab.trial_ends_at IS NOT NULL
		  AND ab.trial_ends_at < $1
		  AND ab.payment_status != 'paid'
		  AND s.status = 'live'
	`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var due []DueTrialPause
	for rows.Next() {
		var d DueTrialPause
		if err := rows.Scan(&d.OwnerUserID, &d.SiteID, &d.Slug, &d.BusinessName, &d.NotifyEmail); err != nil {
			return nil, err
		}
		due = append(due, d)
	}
	return due, rows.Err()
}

func MarkTrialReminderSent(ctx context.Context, q querier, ownerID uuid.UUID, kind string) error {
	var col string
	switch kind {
	case "first":
		col = "trial_reminder_sent_at"
	case "final":
		col = "trial_final_reminder_sent_at"
	case "report_early":
		col = "trial_early_report_sent_at"
	case "report":
		col = "trial_report_sent_at"
	default:
		return fmt.Errorf("unknown trial reminder kind: %s", kind)
	}
	_, err := q.ExecContext(ctx, `UPDATE account_billing SET `+col+` = now() WHERE owner_user_id = $1`, ownerID)
	return err
}

// GetOwnerIDsDueForDunningReminder returns accounts that are past due, whose
// payment_failed_at is at least delay old, and haven't yet received the
// reminder of the given kind ("reminder1", "reminder2", "final_warning") —
// mirrors GetAccountsDueForTrialReminder's shape for the same reason: each
// stage is its own timestamp column rather than a single stage counter, so a
// stage can never be silently skipped by a cron gap.
func GetOwnerIDsDueForDunningReminder(ctx context.Context, q querier, kind string, delay time.Duration) ([]uuid.UUID, error) {
	var col string
	switch kind {
	case "reminder1":
		col = "dunning_reminder_1_sent_at"
	case "reminder2":
		col = "dunning_reminder_2_sent_at"
	case "final_warning":
		col = "dunning_final_warning_sent_at"
	default:
		return nil, fmt.Errorf("unknown dunning reminder kind: %s", kind)
	}
	return scanOwnerIDs(q.QueryContext(ctx, `
		SELECT owner_user_id FROM account_billing
		WHERE payment_status = 'past_due' AND payment_failed_at IS NOT NULL
		  AND payment_failed_at <= $1 AND `+col+` IS NULL
	`, time.Now().UTC().Add(-delay)))
}

func MarkDunningReminderSent(ctx context.Context, q querier, ownerID uuid.UUID, kind string) error {
	var col string
	switch kind {
	case "reminder1":
		col = "dunning_reminder_1_sent_at"
	case "reminder2":
		col = "dunning_reminder_2_sent_at"
	case "final_warning":
		col = "dunning_final_warning_sent_at"
	default:
		return fmt.Errorf("unknown dunning reminder kind: %s", kind)
	}
	_, err := q.ExecContext(ctx, `UPDATE account_billing SET `+col+` = now() WHERE owner_user_id = $1`, ownerID)
	return err
}

// GetOwnerIDsDueForDunningCancellation returns accounts still past due whose
// final warning was sent at least delay ago — the customer had their chance
// to fix payment after the final warning and didn't, so the dunning cron
// cancels these deterministically instead of leaving them past-due forever
// (Stripe's own retry schedule isn't guaranteed to ever give up on its own).
func GetOwnerIDsDueForDunningCancellation(ctx context.Context, q querier, delay time.Duration) ([]uuid.UUID, error) {
	return scanOwnerIDs(q.QueryContext(ctx, `
		SELECT owner_user_id FROM account_billing
		WHERE payment_status = 'past_due' AND dunning_final_warning_sent_at IS NOT NULL
		  AND dunning_final_warning_sent_at <= $1
	`, time.Now().UTC().Add(-delay)))
}

func scanOwnerIDs(rows *sql.Rows, err error) ([]uuid.UUID, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
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

// UnmarkStripeEventProcessed removes a claimed-but-not-completed Stripe event
// record, used to release the claim taken by MarkStripeEventProcessed when
// the handler fails partway through — see HandleWebhookEvent, which claims
// the event ID before processing (so two concurrent duplicate deliveries
// can't both pass the check) and releases the claim on error so Stripe's
// retry isn't permanently skipped.
func UnmarkStripeEventProcessed(ctx context.Context, q querier, eventID string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM stripe_events WHERE event_id = $1`, eventID)
	return err
}

// PruneOldStripeEvents deletes stripe_events rows older than before. These
// only exist to dedupe webhook retries, so they can be dropped well before
// Stripe's retry window (a few days) ever needs them.
func PruneOldStripeEvents(ctx context.Context, q querier, before time.Time) error {
	_, err := q.ExecContext(ctx, `DELETE FROM stripe_events WHERE processed_at < $1`, before)
	return err
}
