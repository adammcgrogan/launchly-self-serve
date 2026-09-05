// Package payment wraps Stripe Checkout for self-serve subscription
// upgrades and webhook handling. There is no admin-sent payment link flow —
// customers create their own checkout session from the dashboard.
package payment

import (
	"encoding/json"
	"fmt"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/stripe/stripe-go/v81"
	portalsession "github.com/stripe/stripe-go/v81/billingportal/session"
	stripesession "github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/subscription"
	"github.com/stripe/stripe-go/v81/webhook"
)

type Client struct {
	webhookSecret string
	starterPrice  string
	proPrice      string
}

func New(secretKey, webhookSecret, starterPriceID, proPriceID string) *Client {
	stripe.Key = secretKey
	return &Client{
		webhookSecret: webhookSecret,
		starterPrice:  starterPriceID,
		proPrice:      proPriceID,
	}
}

func (c *Client) priceForPlan(plan domain.Plan) (string, error) {
	switch plan {
	case domain.PlanStarter:
		return c.starterPrice, nil
	case domain.PlanPro:
		return c.proPrice, nil
	default:
		return "", fmt.Errorf("unknown plan: %s", plan)
	}
}

// CreateCheckoutSession creates a self-serve Stripe Checkout session for a
// plan upgrade and returns the session ID and the hosted checkout URL.
//
// customerID is the site's previously recorded Stripe customer, if any. When
// set it's reused instead of CustomerEmail, so a site that re-subscribes
// after cancelling lands back on the same Stripe customer rather than a new
// one each time — keeping one card, one invoice history, and one customer
// for the billing portal to open. Stripe rejects Customer and CustomerEmail
// together, so it's one or the other.
func (c *Client) CreateCheckoutSession(plan domain.Plan, customerID, customerEmail, successURL, cancelURL string) (sessionID, checkoutURL string, err error) {
	priceID, err := c.priceForPlan(plan)
	if err != nil {
		return "", "", err
	}

	params := &stripe.CheckoutSessionParams{
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{Price: stripe.String(priceID), Quantity: stripe.Int64(1)},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
	}
	if customerID != "" {
		params.Customer = stripe.String(customerID)
	} else {
		params.CustomerEmail = stripe.String(customerEmail)
	}

	sess, err := stripesession.New(params)
	if err != nil {
		return "", "", fmt.Errorf("create checkout session: %w", err)
	}
	return sess.ID, sess.URL, nil
}

// CreateBillingPortalSession opens a Stripe Billing Portal session for a
// customer and returns its hosted URL. The portal is what lets an owner
// update a failing card themselves (see #316) — it also carries invoice
// history and cancellation, so none of that has to be rebuilt here.
//
// The returned URL is single-use and short-lived, so it has to be created
// per click and redirected to immediately — it can't be stored or emailed.
func (c *Client) CreateBillingPortalSession(customerID, returnURL string) (portalURL string, err error) {
	sess, err := portalsession.New(&stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(returnURL),
	})
	if err != nil {
		return "", fmt.Errorf("create billing portal session: %w", err)
	}
	return sess.URL, nil
}

// SubscriptionCustomerID looks up the Stripe customer behind a subscription.
// Used to backfill site_billing.stripe_customer_id for sites that subscribed
// before the ID was persisted — without it those customers, who are exactly
// the ones who may need the portal, have no way into it.
func (c *Client) SubscriptionCustomerID(subscriptionID string) (string, error) {
	sub, err := subscription.Get(subscriptionID, nil)
	if err != nil {
		return "", fmt.Errorf("get subscription: %w", err)
	}
	if sub.Customer == nil {
		return "", fmt.Errorf("subscription %s has no customer", subscriptionID)
	}
	return sub.Customer.ID, nil
}

// ChangeSubscriptionPlan swaps an existing subscription's price to the given
// plan in place, prorating the difference — used for upgrading/downgrading a
// site that's already paid, so the change never stacks a second Stripe
// subscription alongside the original.
func (c *Client) ChangeSubscriptionPlan(subscriptionID string, plan domain.Plan) error {
	priceID, err := c.priceForPlan(plan)
	if err != nil {
		return err
	}
	sub, err := subscription.Get(subscriptionID, nil)
	if err != nil {
		return fmt.Errorf("get subscription: %w", err)
	}
	if len(sub.Items.Data) == 0 {
		return fmt.Errorf("subscription %s has no items", subscriptionID)
	}
	_, err = subscription.Update(subscriptionID, &stripe.SubscriptionParams{
		Items: []*stripe.SubscriptionItemsParams{
			{ID: stripe.String(sub.Items.Data[0].ID), Price: stripe.String(priceID)},
		},
		ProrationBehavior: stripe.String("create_prorations"),
	})
	if err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}

// CancelSubscription immediately cancels a Stripe subscription. If the
// subscription no longer exists in Stripe, it is treated as already cancelled.
func (c *Client) CancelSubscription(subscriptionID string) error {
	_, err := subscription.Cancel(subscriptionID, &stripe.SubscriptionCancelParams{})
	if err != nil {
		if stripeErr, ok := err.(*stripe.Error); ok && stripeErr.Code == stripe.ErrorCodeResourceMissing {
			return nil
		}
		return fmt.Errorf("cancel subscription: %w", err)
	}
	return nil
}

// WebhookEvent is a parsed Stripe webhook event.
type WebhookEvent struct {
	ID             string // Stripe event ID — used for idempotency
	Type           string
	SessionID      string // populated for checkout.session.completed
	SubscriptionID string // populated for checkout.session.completed, customer.subscription.deleted, invoice.payment_failed, invoice.payment_succeeded
	CustomerID     string // populated for every event above — persisted so the billing portal has a customer to open
	CustomerEmail  string // populated for invoice.payment_failed
}

// ParseWebhook verifies the Stripe webhook signature and returns a parsed event.
func (c *Client) ParseWebhook(payload []byte, sigHeader string) (*WebhookEvent, error) {
	event, err := webhook.ConstructEventWithOptions(payload, sigHeader, c.webhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})
	if err != nil {
		return nil, fmt.Errorf("webhook signature: %w", err)
	}
	we := &WebhookEvent{ID: event.ID, Type: string(event.Type)}
	switch event.Type {
	case "checkout.session.completed":
		var sess stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &sess); err != nil {
			return nil, fmt.Errorf("unmarshal session: %w", err)
		}
		we.SessionID = sess.ID
		if sess.Subscription != nil {
			we.SubscriptionID = sess.Subscription.ID
		}
		if sess.Customer != nil {
			we.CustomerID = sess.Customer.ID
		}
	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return nil, fmt.Errorf("unmarshal subscription: %w", err)
		}
		we.SubscriptionID = sub.ID
		if sub.Customer != nil {
			we.CustomerID = sub.Customer.ID
		}
	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			return nil, fmt.Errorf("unmarshal invoice: %w", err)
		}
		if inv.Subscription != nil {
			we.SubscriptionID = inv.Subscription.ID
		}
		if inv.Customer != nil {
			we.CustomerID = inv.Customer.ID
		}
		we.CustomerEmail = inv.CustomerEmail
	case "invoice.payment_succeeded":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			return nil, fmt.Errorf("unmarshal invoice: %w", err)
		}
		if inv.Subscription != nil {
			we.SubscriptionID = inv.Subscription.ID
		}
		if inv.Customer != nil {
			we.CustomerID = inv.Customer.ID
		}
	}
	return we, nil
}
