package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/payment"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// billingMailer is the subset of email.Client's methods billing.go calls.
// Depending on this narrow interface (rather than *email.Client directly)
// lets tests substitute a fake mailer that records calls instead of hitting
// Resend over the network.
type billingMailer interface {
	SendPaymentConfirmation(to, businessName string, plan domain.Plan) error
	SendCancellationConfirmation(to, businessName string) error
	SendPaymentFailed(to, businessName string) error
	SendAdminAlert(to, subject, message string) error
}

// Billing handles self-serve plan upgrades: the customer starts checkout
// from their dashboard, Stripe's webhook confirms payment — there is no
// admin-sent payment link anywhere in this flow.
type Billing struct {
	store   *postgres.Store
	pay     *payment.Client
	mailer  billingMailer
	baseURL string
	sites   *Sites
}

func NewBilling(store *postgres.Store, pay *payment.Client, mailer *email.Client, baseURL string) *Billing {
	return &Billing{store: store, pay: pay, mailer: mailer, baseURL: baseURL}
}

// SetSites wires in the site cache to invalidate after a billing state
// change. It's set post-construction, not passed to NewBilling, because
// NewSites itself takes a *Billing (for CancelSubscriptionIfActive on site
// delete) — a constructor-argument cycle.
func (b *Billing) SetSites(sites *Sites) {
	b.sites = sites
}

// invalidate drops a site's cached aggregate after any write that changes
// its billing state, so the dashboard doesn't show a stale plan/payment
// status for up to siteAggregateCacheTTL afterward.
func (b *Billing) invalidate(siteID int) {
	if b.sites != nil {
		b.sites.invalidateAggregate(siteID)
	}
}

// CreateUpgradeCheckout starts a Stripe Checkout session for a site's plan
// upgrade and records it as pending. If the site already has an active paid
// subscription, there's nothing to check out — the existing subscription is
// changed in place instead, so a plan change never stacks a second Stripe
// subscription and double-bills the customer.
func (b *Billing) CreateUpgradeCheckout(ctx context.Context, siteID int, slug string, plan domain.Plan, customerEmail string) (checkoutURL string, err error) {
	successURL := fmt.Sprintf("%s/dashboard/sites/%s/upgraded", b.baseURL, slug)
	cancelURL := fmt.Sprintf("%s/dashboard/sites/%s", b.baseURL, slug)

	billing, err := postgres.GetSiteBilling(ctx, b.store.DB(), siteID)
	if err != nil {
		return "", fmt.Errorf("load site billing: %w", err)
	}
	if billing != nil && billing.PaymentStatus == domain.PaymentStatusPaid && billing.StripeSubscriptionID != "" {
		if err := b.pay.ChangeSubscriptionPlan(billing.StripeSubscriptionID, plan); err != nil {
			return "", fmt.Errorf("change subscription plan: %w", err)
		}
		if err := b.setSitePlanAfterStripeChange(ctx, siteID, plan, billing.StripeSubscriptionID); err != nil {
			return "", fmt.Errorf("record plan change: %w", err)
		}
		b.invalidate(siteID)
		return successURL, nil
	}

	sessionID, checkoutURL, err := b.pay.CreateCheckoutSession(plan, customerEmail, successURL, cancelURL)
	if err != nil {
		return "", fmt.Errorf("create checkout session: %w", err)
	}
	if err := postgres.SetSitePending(ctx, b.store.DB(), siteID, plan, sessionID); err != nil {
		return "", fmt.Errorf("record pending payment: %w", err)
	}
	b.invalidate(siteID)
	return checkoutURL, nil
}

// setSitePlanAfterStripeChange persists a plan change once Stripe has already
// committed it — that commit can't be cleanly undone, so a transient DB
// failure here would otherwise leave site_billing.plan silently out of sync
// with what the customer is actually being billed. A few retries absorb
// transient errors; if it still fails, this alerts at error level with
// enough detail (site, subscription, target plan) for manual reconciliation.
func (b *Billing) setSitePlanAfterStripeChange(ctx context.Context, siteID int, plan domain.Plan, subscriptionID string) error {
	const maxAttempts = 3
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = postgres.SetSitePlan(ctx, b.store.DB(), siteID, plan); err == nil {
			return nil
		}
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}
	}
	slog.Error("stripe/db plan desync: subscription updated but db write failed after retries",
		"site_id", siteID, "subscription_id", subscriptionID, "target_plan", plan, "error", err)
	b.mailer.SendAdminAlert(
		"hello@launchly.ltd",
		"Stripe/DB plan desync needs manual reconciliation",
		fmt.Sprintf("Site <strong>%d</strong> subscription <strong>%s</strong> was changed to <strong>%s</strong> in Stripe, but the local plan record failed to update after %d attempts: %v. site_billing.plan is now out of sync with Stripe and needs manual correction.", siteID, subscriptionID, plan, maxAttempts, err),
	)
	return err
}

// ParseWebhook verifies the Stripe webhook signature and returns a parsed
// event, keeping the payment package's types out of the web layer.
func (b *Billing) ParseWebhook(payload []byte, sigHeader string) (*payment.WebhookEvent, error) {
	return b.pay.ParseWebhook(payload, sigHeader)
}

// HandleWebhookEvent processes a verified Stripe webhook event, idempotently.
// The event ID is claimed *before* processing (an atomic INSERT ... ON
// CONFLICT DO NOTHING): only one of two concurrent duplicate deliveries can
// win the insert, so the second is skipped without ever running the handler.
// That closes the race the old check-then-mark-after-success sequence had —
// two close-together retries of the same event could both pass the "is it
// processed" check before either recorded it, each running
// handleSubscriptionDeleted/handlePaymentFailed and re-sending the
// cancellation/payment-failed emails.
//
// If processing then fails, the claim is released (the stripe_events row is
// deleted) so Stripe's automatic retry isn't permanently skipped — a
// transient error mid-handler doesn't lose the payment/cancellation update.
func (b *Billing) HandleWebhookEvent(ctx context.Context, event *payment.WebhookEvent) error {
	if event.ID != "" {
		claimed, err := postgres.MarkStripeEventProcessed(ctx, b.store.DB(), event.ID)
		if err != nil {
			return fmt.Errorf("claim event idempotency: %w", err)
		}
		if !claimed {
			slog.Info("stripe event already processed or in flight, skipping", "event_id", event.ID)
			return nil
		}
	}

	var err error
	switch event.Type {
	case "checkout.session.completed":
		err = b.handleCheckoutCompleted(ctx, event)
	case "customer.subscription.deleted":
		err = b.handleSubscriptionDeleted(ctx, event)
	case "invoice.payment_failed":
		err = b.handlePaymentFailed(ctx, event)
	case "invoice.payment_succeeded":
		err = b.handlePaymentRecovered(ctx, event)
	}
	if err != nil {
		if event.ID != "" {
			if unmarkErr := postgres.UnmarkStripeEventProcessed(ctx, b.store.DB(), event.ID); unmarkErr != nil {
				slog.Error("release stripe event claim after failed processing", "event_id", event.ID, "error", unmarkErr)
			}
		}
		return err
	}
	return nil
}

func (b *Billing) handleCheckoutCompleted(ctx context.Context, event *payment.WebhookEvent) error {
	if event.SessionID == "" {
		return nil
	}
	first, err := postgres.SetSitePaid(ctx, b.store.DB(), event.SessionID, event.SubscriptionID)
	if err != nil {
		return fmt.Errorf("set site paid: %w", err)
	}
	slog.Info("payment received", "session_id", event.SessionID, "first", first)
	if !first {
		return nil
	}

	billing, err := postgres.GetSiteBillingBySessionID(ctx, b.store.DB(), event.SessionID)
	if err != nil || billing == nil {
		return err
	}
	b.invalidate(billing.SiteID)
	site, to, err := resolveNotifyTarget(ctx, b.store, billing.SiteID)
	if err != nil || site == nil {
		return err
	}
	if site.Status == domain.SiteStatusPaused {
		if err := postgres.SetSiteStatus(ctx, b.store.DB(), site.ID, domain.SiteStatusLive); err != nil {
			slog.Error("reactivate paused site", "site_id", site.ID, "error", err)
		}
	}
	if to == "" {
		return nil
	}
	if err := b.mailer.SendPaymentConfirmation(to, site.BusinessName, billing.Plan); err != nil {
		slog.Error("send payment confirmation email", "error", err)
	}
	return nil
}

func (b *Billing) handleSubscriptionDeleted(ctx context.Context, event *payment.WebhookEvent) error {
	if event.SubscriptionID == "" {
		return nil
	}
	billing, err := postgres.GetSiteBillingBySubscriptionID(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil {
		slog.Error("lookup site billing by subscription id", "subscription_id", event.SubscriptionID, "error", err)
		b.mailer.SendAdminAlert(
			"hello@launchly.ltd",
			"Subscription cancellation lookup failed",
			fmt.Sprintf("Looking up site billing for subscription <strong>%s</strong> failed: %v. The subscription was still marked cancelled, but the owner may not have been notified.", event.SubscriptionID, err),
		)
	}
	if err := postgres.SetSiteCancelled(ctx, b.store.DB(), event.SubscriptionID); err != nil {
		return fmt.Errorf("set site cancelled: %w", err)
	}
	slog.Info("subscription cancelled", "subscription_id", event.SubscriptionID)
	if billing == nil {
		return nil
	}
	b.invalidate(billing.SiteID)
	site, to, _ := resolveNotifyTarget(ctx, b.store, billing.SiteID)
	if site == nil {
		return nil
	}
	if site.Status == domain.SiteStatusLive {
		if err := postgres.SetSiteStatus(ctx, b.store.DB(), site.ID, domain.SiteStatusPaused); err != nil {
			slog.Error("pause site on subscription cancellation", "site_id", site.ID, "error", err)
		}
	}
	if to != "" {
		if err := b.mailer.SendCancellationConfirmation(to, site.BusinessName); err != nil {
			slog.Error("send cancellation confirmation email", "error", err)
		}
	}
	b.mailer.SendAdminAlert(
		"hello@launchly.ltd",
		fmt.Sprintf("Subscription cancelled - %s", site.BusinessName),
		fmt.Sprintf("<strong>%s</strong> has cancelled their subscription (or payment ultimately failed).", site.BusinessName),
	)
	return nil
}

// handlePaymentFailed starts the dunning sequence the first time a payment
// fails for a subscription (see postgres.SetSitePaymentFailed). Stripe sends
// invoice.payment_failed on every retry attempt it makes for the same
// underlying failure, not just once — first is guarded on so a business
// already mid-sequence doesn't get the "payment failed" email and admin
// alert repeated on every one of Stripe's own retries; the dunning cron
// (service.Cron.sendDueDunningReminders) takes over the follow-ups from here.
func (b *Billing) handlePaymentFailed(ctx context.Context, event *payment.WebhookEvent) error {
	if event.SubscriptionID == "" {
		return nil
	}
	billing, err := postgres.GetSiteBillingBySubscriptionID(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil {
		slog.Error("lookup site billing by subscription id", "subscription_id", event.SubscriptionID, "error", err)
		b.mailer.SendAdminAlert(
			"hello@launchly.ltd",
			"Payment failure lookup failed",
			fmt.Sprintf("Looking up site billing for subscription <strong>%s</strong> failed after a payment failure event: %v. The owner may not have been notified.", event.SubscriptionID, err),
		)
	}
	slog.Warn("payment failed", "subscription_id", event.SubscriptionID)
	if billing == nil {
		return nil
	}
	first, err := postgres.SetSitePaymentFailed(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil {
		return fmt.Errorf("set site payment failed: %w", err)
	}
	if !first {
		slog.Info("payment failed, already in dunning sequence", "subscription_id", event.SubscriptionID)
		return nil
	}
	b.invalidate(billing.SiteID)
	site, to, _ := resolveNotifyTarget(ctx, b.store, billing.SiteID)
	if site == nil {
		return nil
	}
	if to != "" {
		if err := b.mailer.SendPaymentFailed(to, site.BusinessName); err != nil {
			slog.Error("send payment failed email", "error", err)
		}
	}
	b.mailer.SendAdminAlert(
		"hello@launchly.ltd",
		fmt.Sprintf("Payment failed - %s", site.BusinessName),
		fmt.Sprintf("A monthly payment has failed for <strong>%s</strong> (%s). It's now in the dunning sequence — reminders will escalate over the next week before the subscription is cancelled if unresolved.", site.BusinessName, to),
	)
	return nil
}

// handlePaymentRecovered moves a site back out of the dunning sequence once
// Stripe confirms an invoice succeeded. Stripe sends invoice.payment_succeeded
// for every successful invoice, including routine renewals of an
// already-'paid' subscription, so SetSitePaymentRecovered only acts (and this
// only notifies) when the site was actually 'past_due' — anything else is a
// no-op.
func (b *Billing) handlePaymentRecovered(ctx context.Context, event *payment.WebhookEvent) error {
	if event.SubscriptionID == "" {
		return nil
	}
	recovered, err := postgres.SetSitePaymentRecovered(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil {
		return fmt.Errorf("set site payment recovered: %w", err)
	}
	if !recovered {
		return nil
	}
	slog.Info("payment recovered", "subscription_id", event.SubscriptionID)
	billing, err := postgres.GetSiteBillingBySubscriptionID(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil || billing == nil {
		return err
	}
	b.invalidate(billing.SiteID)
	site, to, err := resolveNotifyTarget(ctx, b.store, billing.SiteID)
	if err != nil || site == nil {
		return err
	}
	if to != "" {
		if err := b.mailer.SendPaymentConfirmation(to, site.BusinessName, billing.Plan); err != nil {
			slog.Error("send payment recovered email", "error", err)
		}
	}
	return nil
}

func (b *Billing) CancelSubscription(ctx context.Context, siteID int) error {
	billing, err := postgres.GetSiteBilling(ctx, b.store.DB(), siteID)
	if err != nil {
		return err
	}
	if billing == nil || billing.StripeSubscriptionID == "" {
		return fmt.Errorf("no subscription on record")
	}
	if err := b.pay.CancelSubscription(billing.StripeSubscriptionID); err != nil {
		return err
	}
	if err := postgres.SetSiteCancelled(ctx, b.store.DB(), billing.StripeSubscriptionID); err != nil {
		return err
	}
	b.invalidate(siteID)
	site, err := postgres.GetSiteByID(ctx, b.store.DB(), siteID)
	if err != nil {
		return err
	}
	if site != nil && site.Status == domain.SiteStatusLive {
		return postgres.SetSiteStatus(ctx, b.store.DB(), siteID, domain.SiteStatusPaused)
	}
	return nil
}

// CancelSubscriptionIfActive cancels a site's Stripe subscription if one
// exists, and is a no-op (not an error) if the site was never upgraded — so
// callers like Sites.Delete can call it unconditionally before a site (and
// its billing row) disappears, instead of leaving an orphaned subscription
// that keeps charging a customer for a site that no longer exists.
func (b *Billing) CancelSubscriptionIfActive(ctx context.Context, siteID int) error {
	billing, err := postgres.GetSiteBilling(ctx, b.store.DB(), siteID)
	if err != nil {
		return err
	}
	if billing == nil || billing.StripeSubscriptionID == "" {
		return nil
	}
	if err := b.pay.CancelSubscription(billing.StripeSubscriptionID); err != nil {
		return err
	}
	return postgres.SetSiteCancelled(ctx, b.store.DB(), billing.StripeSubscriptionID)
}
