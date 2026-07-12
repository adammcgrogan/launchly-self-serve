package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/payment"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// Billing handles self-serve plan upgrades: the customer starts checkout
// from their dashboard, Stripe's webhook confirms payment — there is no
// admin-sent payment link anywhere in this flow.
type Billing struct {
	store   *postgres.Store
	pay     *payment.Client
	mailer  *email.Client
	baseURL string
}

func NewBilling(store *postgres.Store, pay *payment.Client, mailer *email.Client, baseURL string) *Billing {
	return &Billing{store: store, pay: pay, mailer: mailer, baseURL: baseURL}
}

// CreateUpgradeCheckout starts a Stripe Checkout session for a site's plan
// upgrade and records it as pending.
func (b *Billing) CreateUpgradeCheckout(ctx context.Context, siteID int, plan domain.Plan, customerEmail string) (checkoutURL string, err error) {
	successURL := fmt.Sprintf("%s/dashboard/sites/%d?upgraded=1", b.baseURL, siteID)
	cancelURL := fmt.Sprintf("%s/dashboard/sites/%d", b.baseURL, siteID)

	sessionID, checkoutURL, err := b.pay.CreateCheckoutSession(plan, customerEmail, successURL, cancelURL)
	if err != nil {
		return "", fmt.Errorf("create checkout session: %w", err)
	}
	if err := postgres.SetSitePending(ctx, b.store.DB(), siteID, plan, sessionID); err != nil {
		return "", fmt.Errorf("record pending payment: %w", err)
	}
	return checkoutURL, nil
}

// ParseWebhook verifies the Stripe webhook signature and returns a parsed
// event, keeping the payment package's types out of the web layer.
func (b *Billing) ParseWebhook(payload []byte, sigHeader string) (*payment.WebhookEvent, error) {
	return b.pay.ParseWebhook(payload, sigHeader)
}

// HandleWebhookEvent processes a verified Stripe webhook event, idempotently.
// The event is only marked processed *after* it's handled successfully — if
// we marked it first and the handler then failed on a transient error,
// Stripe's retry would see the event as already processed and skip it,
// permanently losing that payment/cancellation update.
func (b *Billing) HandleWebhookEvent(ctx context.Context, event *payment.WebhookEvent) error {
	if event.ID != "" {
		processed, err := postgres.IsStripeEventProcessed(ctx, b.store.DB(), event.ID)
		if err != nil {
			return fmt.Errorf("check event idempotency: %w", err)
		}
		if processed {
			slog.Info("stripe event already processed, skipping", "event_id", event.ID)
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
	}
	if err != nil {
		return err
	}

	if event.ID != "" {
		if _, err := postgres.MarkStripeEventProcessed(ctx, b.store.DB(), event.ID); err != nil {
			slog.Error("mark stripe event processed", "event_id", event.ID, "error", err)
		}
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
	site, err := postgres.GetSiteByID(ctx, b.store.DB(), billing.SiteID)
	if err != nil || site == nil {
		return err
	}
	if site.Status == domain.SiteStatusPaused {
		if err := postgres.SetSiteStatus(ctx, b.store.DB(), site.ID, domain.SiteStatusLive); err != nil {
			slog.Error("reactivate paused site", "site_id", site.ID, "error", err)
		}
	}
	contact, err := postgres.GetSiteContact(ctx, b.store.DB(), billing.SiteID)
	if err != nil {
		return err
	}
	contactEmail := ""
	if contact != nil {
		contactEmail = contact.Email
	}
	to := notifyEmail(ctx, b.store, site.OwnerUserID, contactEmail)
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
	billing, _ := postgres.GetSiteBillingBySubscriptionID(ctx, b.store.DB(), event.SubscriptionID)
	if err := postgres.SetSiteCancelled(ctx, b.store.DB(), event.SubscriptionID); err != nil {
		return fmt.Errorf("set site cancelled: %w", err)
	}
	slog.Info("subscription cancelled", "subscription_id", event.SubscriptionID)
	if billing == nil {
		return nil
	}
	site, _ := postgres.GetSiteByID(ctx, b.store.DB(), billing.SiteID)
	contact, _ := postgres.GetSiteContact(ctx, b.store.DB(), billing.SiteID)
	if site == nil {
		return nil
	}
	contactEmail := ""
	if contact != nil {
		contactEmail = contact.Email
	}
	if to := notifyEmail(ctx, b.store, site.OwnerUserID, contactEmail); to != "" {
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

func (b *Billing) handlePaymentFailed(ctx context.Context, event *payment.WebhookEvent) error {
	if event.SubscriptionID == "" {
		return nil
	}
	billing, _ := postgres.GetSiteBillingBySubscriptionID(ctx, b.store.DB(), event.SubscriptionID)
	slog.Warn("payment failed", "subscription_id", event.SubscriptionID)
	if billing == nil {
		return nil
	}
	site, _ := postgres.GetSiteByID(ctx, b.store.DB(), billing.SiteID)
	contact, _ := postgres.GetSiteContact(ctx, b.store.DB(), billing.SiteID)
	if site == nil {
		return nil
	}
	contactEmail := ""
	if contact != nil {
		contactEmail = contact.Email
	}
	to := notifyEmail(ctx, b.store, site.OwnerUserID, contactEmail)
	if to != "" {
		if err := b.mailer.SendPaymentFailed(to, site.BusinessName); err != nil {
			slog.Error("send payment failed email", "error", err)
		}
	}
	b.mailer.SendAdminAlert(
		"hello@launchly.ltd",
		fmt.Sprintf("Payment failed - %s", site.BusinessName),
		fmt.Sprintf("A monthly payment has failed for <strong>%s</strong> (%s). Stripe will retry automatically.", site.BusinessName, to),
	)
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
	return postgres.SetSiteCancelled(ctx, b.store.DB(), billing.StripeSubscriptionID)
}
