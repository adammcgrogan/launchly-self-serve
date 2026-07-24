package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// advisoryLockDunningCron serializes the dunning sweep across server
// instances, the same way advisoryLockTrialCron does for trial reminders —
// see withAdvisoryLock. A separate key from the trial lock so the two
// sweeps don't needlessly serialize against each other.
const advisoryLockDunningCron = 8735102

// Dunning schedule, anchored to site_billing.payment_failed_at (the moment a
// payment first failed for a subscription — see Billing.handlePaymentFailed
// / postgres.SetSitePaymentFailed). This is independent of how many times
// Stripe itself retries the underlying charge, so the sequence always
// escalates on a predictable timeline even if Stripe's retries land at
// irregular intervals.
const (
	dunningReminder1Delay    = 24 * time.Hour     // day 1: first follow-up
	dunningReminder2Delay    = 3 * 24 * time.Hour // day 3: second follow-up
	dunningFinalWarningDelay = 7 * 24 * time.Hour // day 7: last warning before cancellation
	dunningCancelDelay       = 24 * time.Hour     // 1 day after the final warning, if still unresolved
)

// sendDueDunningReminders sends the escalating dunning emails (day 1, day 3,
// day 7) for sites that have been past due long enough to be due for the
// next stage and haven't received it yet. Mirrors sendDueTrialReminders'
// shape: each stage is checked and marked independently, so a stage is never
// silently skipped by a cron gap, and a site already past the later stage
// still gets exactly one email per pass (the loop just runs all due stages).
func (c *Cron) sendDueDunningReminders() {
	ctx := context.Background()
	c.withAdvisoryLock(ctx, advisoryLockDunningCron, func(conn *sql.Conn) {
		stages := []struct {
			kind  string
			delay time.Duration
		}{
			{"reminder1", dunningReminder1Delay},
			{"reminder2", dunningReminder2Delay},
			{"final_warning", dunningFinalWarningDelay},
		}
		for _, stage := range stages {
			ids, err := postgres.GetSiteIDsDueForDunningReminder(ctx, conn, stage.kind, stage.delay)
			if err != nil {
				slog.Error("dunning cron: list sites", "kind", stage.kind, "error", err)
				continue
			}
			for _, id := range ids {
				c.sendDunningStage(ctx, conn, id, stage.kind)
			}
		}
	})
}

func (c *Cron) sendDunningStage(ctx context.Context, conn *sql.Conn, siteID int, kind string) {
	site, err := postgres.GetSiteByID(ctx, conn, siteID)
	if err != nil || site == nil {
		return
	}
	billing, err := postgres.GetSiteBilling(ctx, conn, siteID)
	if err != nil || billing == nil || billing.PaymentFailedAt == nil {
		return
	}
	contact, err := postgres.GetSiteContact(ctx, conn, siteID)
	if err != nil {
		return
	}
	contactEmail := ""
	if contact != nil {
		contactEmail = contact.Email
	}
	to := notifyEmail(ctx, c.store, site.OwnerUserID, contactEmail)
	if to == "" {
		return
	}
	dashboardURL := fmt.Sprintf("%s/dashboard/sites/%s?tab=billing", c.baseURL, site.Slug)
	daysPastDue := int(time.Since(*billing.PaymentFailedAt).Hours() / 24)
	if daysPastDue < 1 {
		daysPastDue = 1
	}

	var sendErr error
	switch kind {
	case "reminder1", "reminder2":
		sendErr = c.mailer.SendDunningReminder(to, site.BusinessName, dashboardURL, daysPastDue)
	case "final_warning":
		sendErr = c.mailer.SendFinalPaymentWarning(to, site.BusinessName, dashboardURL)
	}
	if sendErr != nil {
		slog.Error("dunning cron: send reminder", "slug", site.Slug, "kind", kind, "error", sendErr)
		return
	}
	if err := postgres.MarkDunningReminderSent(ctx, conn, siteID, kind); err != nil {
		slog.Error("dunning cron: mark sent", "slug", site.Slug, "kind", kind, "error", err)
		return
	}
	slog.Info("dunning reminder sent", "slug", site.Slug, "kind", kind)
}

// cancelOverdueDunningSites cancels the subscription for any site that's
// still past due a full day after its final warning — the customer had the
// entire escalating sequence (day 1, day 3, day 7, plus this extra day) to
// fix payment and didn't. Cancellation goes through Billing.CancelSubscription
// so it's identical to a self-serve cancel: Stripe's own
// customer.subscription.deleted webhook is what actually marks the site
// cancelled/paused and sends the cancellation email
// (Billing.handleSubscriptionDeleted) — this just triggers it, rather than
// duplicating that bookkeeping and risking a double-sent email.
func (c *Cron) cancelOverdueDunningSites() {
	ctx := context.Background()
	c.withAdvisoryLock(ctx, advisoryLockDunningCron, func(conn *sql.Conn) {
		ids, err := postgres.GetSiteIDsDueForDunningCancellation(ctx, conn, dunningCancelDelay)
		if err != nil {
			slog.Error("dunning cron: list sites due for cancellation", "error", err)
			return
		}
		for _, id := range ids {
			site, err := postgres.GetSiteByID(ctx, conn, id)
			if err != nil || site == nil {
				continue
			}
			if err := c.billing.CancelSubscription(ctx, id); err != nil {
				slog.Error("dunning cron: cancel overdue subscription", "slug", site.Slug, "error", err)
				continue
			}
			slog.Info("dunning cron: cancelled overdue subscription", "slug", site.Slug)
		}
	})
}
