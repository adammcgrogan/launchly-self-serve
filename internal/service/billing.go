package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/payment"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

// billingMailer is the subset of email.Client's methods billing.go calls.
// Depending on this narrow interface (rather than *email.Client directly)
// lets tests substitute a fake mailer that records calls instead of hitting
// Resend over the network.
type billingMailer interface {
	SendPaymentConfirmation(to, businessName string, plan domain.Plan) error
	SendCancellationConfirmation(to, businessName string) error
	SendPaymentFailed(to, businessName, billingURL string) error
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

// invalidateOwner is invalidate for every site an account owns. Billing is
// per-account (#338), so one payment event changes the plan shown on all of
// them — invalidating only the site that happened to be in hand would leave
// the others serving a stale plan until the cache expired.
func (b *Billing) invalidateOwner(ctx context.Context, ownerID uuid.UUID) {
	if b.sites == nil {
		return
	}
	ids, err := postgres.ListSiteIDsByOwner(ctx, b.store.DB(), ownerID)
	if err != nil {
		slog.Error("list sites for cache invalidation", "owner_user_id", ownerID, "error", err)
		return
	}
	for _, id := range ids {
		b.sites.invalidateAggregate(id)
	}
}

// billingURL is the dashboard billing page for a site — where the billing
// portal button lives, and so where every payment email points.
func (b *Billing) billingURL(slug string) string {
	return fmt.Sprintf("%s/dashboard/sites/%s/billing", b.baseURL, slug)
}

// CreateUpgradeCheckout starts a Stripe Checkout session for an account's
// plan upgrade and records it as pending. One subscription covers every site
// the account owns (#338). If the account already has an active paid
// subscription, there's nothing to check out — the existing subscription is
// changed in place instead, so a plan change never stacks a second Stripe
// subscription and double-bills the customer.
//
// slug is only used to build the return URLs, so the customer lands back on
// the site they started from.
func (b *Billing) CreateUpgradeCheckout(ctx context.Context, ownerID uuid.UUID, slug string, plan domain.Plan, customerEmail string) (checkoutURL string, err error) {
	successURL := fmt.Sprintf("%s/dashboard/sites/%s/upgraded", b.baseURL, slug)
	cancelURL := fmt.Sprintf("%s/dashboard/sites/%s", b.baseURL, slug)

	billing, err := postgres.GetAccountBilling(ctx, b.store.DB(), ownerID)
	if err != nil {
		return "", fmt.Errorf("load account billing: %w", err)
	}
	if billing != nil && billing.PaymentStatus == domain.PaymentStatusPaid && billing.StripeSubscriptionID != "" {
		if err := b.pay.ChangeSubscriptionPlan(billing.StripeSubscriptionID, plan); err != nil {
			return "", fmt.Errorf("change subscription plan: %w", err)
		}
		if err := b.setAccountPlanAfterStripeChange(ctx, ownerID, plan, billing.StripeSubscriptionID); err != nil {
			return "", fmt.Errorf("record plan change: %w", err)
		}
		b.invalidateOwner(ctx, ownerID)
		return successURL, nil
	}

	existingCustomerID := ""
	if billing != nil {
		existingCustomerID = billing.StripeCustomerID
	}
	sessionID, checkoutURL, err := b.pay.CreateCheckoutSession(plan, existingCustomerID, customerEmail, successURL, cancelURL)
	if err != nil {
		return "", fmt.Errorf("create checkout session: %w", err)
	}
	if err := postgres.SetAccountPending(ctx, b.store.DB(), ownerID, plan, sessionID); err != nil {
		return "", fmt.Errorf("record pending payment: %w", err)
	}
	b.invalidateOwner(ctx, ownerID)
	return checkoutURL, nil
}

// ErrNoBillingCustomer means an account has no Stripe customer to open the
// billing portal against — it never completed a checkout, so there is no
// card or invoice history to manage yet. Callers surface this as "upgrade
// first" rather than a server error.
var ErrNoBillingCustomer = errors.New("account has no stripe customer")

// CreateBillingPortalSession opens a Stripe Billing Portal session for an
// account and returns the URL to redirect to. This is the self-serve
// path for updating a failing card (see #316) — before it existed, a
// past-due customer's only options on the billing page were to email support
// or cancel, so the dunning sequence ran to cancellation on customers who
// wanted to keep paying.
func (b *Billing) CreateBillingPortalSession(ctx context.Context, ownerID uuid.UUID, slug string) (portalURL string, err error) {
	customerID, err := b.stripeCustomerID(ctx, ownerID)
	if err != nil {
		return "", err
	}
	returnURL := fmt.Sprintf("%s/dashboard/sites/%s/billing", b.baseURL, slug)
	return b.pay.CreateBillingPortalSession(customerID, returnURL)
}

// stripeCustomerID resolves an account's Stripe customer, backfilling it
// from the subscription if it was never recorded — nothing wrote the column
// until #316, so an account that subscribed before then has an empty one,
// and those are exactly the customers whose card may now be failing.
func (b *Billing) stripeCustomerID(ctx context.Context, ownerID uuid.UUID) (string, error) {
	billing, err := postgres.GetAccountBilling(ctx, b.store.DB(), ownerID)
	if err != nil {
		return "", fmt.Errorf("load account billing: %w", err)
	}
	if billing == nil {
		return "", ErrNoBillingCustomer
	}
	if billing.StripeCustomerID != "" {
		return billing.StripeCustomerID, nil
	}
	if billing.StripeSubscriptionID == "" {
		return "", ErrNoBillingCustomer
	}
	customerID, err := b.pay.SubscriptionCustomerID(billing.StripeSubscriptionID)
	if err != nil {
		return "", fmt.Errorf("backfill stripe customer: %w", err)
	}
	// A failure to persist isn't fatal — the portal still opens, it just
	// costs another Stripe lookup next time.
	if err := postgres.SetStripeCustomerID(ctx, b.store.DB(), ownerID, customerID); err != nil {
		slog.Error("persist backfilled stripe customer", "owner_user_id", ownerID, "error", err)
	}
	return customerID, nil
}

// setAccountPlanAfterStripeChange persists a plan change once Stripe has
// already committed it — that commit can't be cleanly undone, so a transient
// DB failure here would otherwise leave account_billing.plan silently out of
// sync with what the customer is actually being billed. A few retries absorb
// transient errors; if it still fails, this alerts at error level with
// enough detail (site, subscription, target plan) for manual reconciliation.
func (b *Billing) setAccountPlanAfterStripeChange(ctx context.Context, ownerID uuid.UUID, plan domain.Plan, subscriptionID string) error {
	const maxAttempts = 3
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = postgres.SetAccountPlan(ctx, b.store.DB(), ownerID, plan); err == nil {
			return nil
		}
		if attempt < maxAttempts {
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
		}
	}
	slog.Error("stripe/db plan desync: subscription updated but db write failed after retries",
		"owner_user_id", ownerID, "subscription_id", subscriptionID, "target_plan", plan, "error", err)
	b.mailer.SendAdminAlert(
		"hello@launchly.ltd",
		"Stripe/DB plan desync needs manual reconciliation",
		fmt.Sprintf("Account <strong>%s</strong> subscription <strong>%s</strong> was changed to <strong>%s</strong> in Stripe, but the local plan record failed to update after %d attempts: %v. account_billing.plan is now out of sync with Stripe and needs manual correction.", ownerID, subscriptionID, plan, maxAttempts, err),
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
	first, err := postgres.SetAccountPaid(ctx, b.store.DB(), event.SessionID, event.SubscriptionID, event.CustomerID)
	if err != nil {
		return fmt.Errorf("set account paid: %w", err)
	}
	slog.Info("payment received", "session_id", event.SessionID, "first", first)
	if !first {
		return nil
	}

	billing, err := postgres.GetAccountBillingBySessionID(ctx, b.store.DB(), event.SessionID)
	if err != nil || billing == nil {
		return err
	}
	// Paying brings back every site the account had paused for non-payment,
	// not just one — the subscription covers the whole account (#338).
	if err := postgres.ReactivateSitesByOwner(ctx, b.store.DB(), billing.OwnerUserID); err != nil {
		slog.Error("reactivate paused sites", "owner_user_id", billing.OwnerUserID, "error", err)
	}
	b.invalidateOwner(ctx, billing.OwnerUserID)
	site, to, err := resolveAccountNotifyTarget(ctx, b.store, billing.OwnerUserID)
	if err != nil || site == nil {
		return err
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
	billing, err := postgres.GetAccountBillingBySubscriptionID(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil {
		slog.Error("lookup account billing by subscription id", "subscription_id", event.SubscriptionID, "error", err)
		b.mailer.SendAdminAlert(
			"hello@launchly.ltd",
			"Subscription cancellation lookup failed",
			fmt.Sprintf("Looking up account billing for subscription <strong>%s</strong> failed: %v. The subscription was still marked cancelled, but the owner may not have been notified.", event.SubscriptionID, err),
		)
	}
	if err := postgres.SetAccountCancelled(ctx, b.store.DB(), event.SubscriptionID); err != nil {
		return fmt.Errorf("set account cancelled: %w", err)
	}
	slog.Info("subscription cancelled", "subscription_id", event.SubscriptionID)
	if billing == nil {
		return nil
	}
	// One subscription covers the account, so losing it takes every live site
	// down together (#338) — that's the deal the customer agreed to, and the
	// alternative is a paid-for account with silently unpaid sites.
	if err := postgres.SetSitesPausedByOwner(ctx, b.store.DB(), billing.OwnerUserID); err != nil {
		slog.Error("pause sites on subscription cancellation", "owner_user_id", billing.OwnerUserID, "error", err)
	}
	b.invalidateOwner(ctx, billing.OwnerUserID)
	site, to, _ := resolveAccountNotifyTarget(ctx, b.store, billing.OwnerUserID)
	if site == nil {
		return nil
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
	billing, err := postgres.GetAccountBillingBySubscriptionID(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil {
		slog.Error("lookup account billing by subscription id", "subscription_id", event.SubscriptionID, "error", err)
		b.mailer.SendAdminAlert(
			"hello@launchly.ltd",
			"Payment failure lookup failed",
			fmt.Sprintf("Looking up account billing for subscription <strong>%s</strong> failed after a payment failure event: %v. The owner may not have been notified.", event.SubscriptionID, err),
		)
	}
	slog.Warn("payment failed", "subscription_id", event.SubscriptionID)
	if billing == nil {
		return nil
	}
	first, err := postgres.SetAccountPaymentFailed(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil {
		return fmt.Errorf("set account payment failed: %w", err)
	}
	if !first {
		slog.Info("payment failed, already in dunning sequence", "subscription_id", event.SubscriptionID)
		return nil
	}
	b.invalidateOwner(ctx, billing.OwnerUserID)
	// A past-due account is the one that most needs the billing portal, so
	// make sure the customer it opens against is on record before the dunning
	// emails start pointing people at it.
	if event.CustomerID != "" && billing.StripeCustomerID == "" {
		if err := postgres.SetStripeCustomerID(ctx, b.store.DB(), billing.OwnerUserID, event.CustomerID); err != nil {
			slog.Error("record stripe customer on payment failure", "owner_user_id", billing.OwnerUserID, "error", err)
		}
	}
	site, to, _ := resolveAccountNotifyTarget(ctx, b.store, billing.OwnerUserID)
	if site == nil {
		return nil
	}
	if to != "" {
		if err := b.mailer.SendPaymentFailed(to, site.BusinessName, b.billingURL(site.Slug)); err != nil {
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
	recovered, err := postgres.SetAccountPaymentRecovered(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil {
		return fmt.Errorf("set account payment recovered: %w", err)
	}
	if !recovered {
		return nil
	}
	slog.Info("payment recovered", "subscription_id", event.SubscriptionID)
	billing, err := postgres.GetAccountBillingBySubscriptionID(ctx, b.store.DB(), event.SubscriptionID)
	if err != nil || billing == nil {
		return err
	}
	b.invalidateOwner(ctx, billing.OwnerUserID)
	site, to, err := resolveAccountNotifyTarget(ctx, b.store, billing.OwnerUserID)
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

// CancelSubscription cancels an account's subscription and takes every site
// it owns offline. One subscription covers the whole account (#338), so this
// is deliberately all-or-nothing — there is no way to stop paying for one
// site while keeping another.
func (b *Billing) CancelSubscription(ctx context.Context, ownerID uuid.UUID) error {
	billing, err := postgres.GetAccountBilling(ctx, b.store.DB(), ownerID)
	if err != nil {
		return err
	}
	if billing == nil || billing.StripeSubscriptionID == "" {
		return fmt.Errorf("no subscription on record")
	}
	if err := b.pay.CancelSubscription(billing.StripeSubscriptionID); err != nil {
		return err
	}
	if err := postgres.SetAccountCancelled(ctx, b.store.DB(), billing.StripeSubscriptionID); err != nil {
		return err
	}
	if err := postgres.SetSitesPausedByOwner(ctx, b.store.DB(), ownerID); err != nil {
		return err
	}
	b.invalidateOwner(ctx, ownerID)
	return nil
}

// CancelSubscriptionIfLastSite cancels an account's Stripe subscription when
// the site being deleted is the last one it owns, and is a no-op otherwise.
// Sites.Delete calls it before removing a site: with per-account billing
// (#338) deleting one of several sites must leave the subscription running
// for the rest, but deleting the last one would otherwise keep charging a
// customer with nothing left to pay for and no dashboard page to cancel from.
//
// siteID is still counted at this point, so "last site" means a count of 1.
func (b *Billing) CancelSubscriptionIfLastSite(ctx context.Context, ownerID uuid.UUID) error {
	count, err := postgres.CountSitesByOwner(ctx, b.store.DB(), ownerID)
	if err != nil {
		return err
	}
	if count > 1 {
		return nil
	}
	billing, err := postgres.GetAccountBilling(ctx, b.store.DB(), ownerID)
	if err != nil {
		return err
	}
	if billing == nil || billing.StripeSubscriptionID == "" {
		return nil
	}
	if err := b.pay.CancelSubscription(billing.StripeSubscriptionID); err != nil {
		return err
	}
	return postgres.SetAccountCancelled(ctx, b.store.DB(), billing.StripeSubscriptionID)
}
