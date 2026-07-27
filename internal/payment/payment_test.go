package payment

import (
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stripe/stripe-go/v81/webhook"
)

const testWebhookSecret = "whsec_test_secret"

// signedPayload builds a Stripe event JSON payload and a valid
// Stripe-Signature header for it, the same way Stripe itself signs webhook
// deliveries, so ParseWebhook's signature verification can be exercised
// without hitting the network.
func signedPayload(t *testing.T, eventID, eventType, dataObjectJSON string) (payload []byte, sigHeader string) {
	t.Helper()
	payload = []byte(fmt.Sprintf(`{"id":%q,"object":"event","type":%q,"data":{"object":%s}}`, eventID, eventType, dataObjectJSON))
	ts := time.Now()
	sig := webhook.ComputeSignature(ts, payload, testWebhookSecret)
	sigHeader = fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(sig))
	return payload, sigHeader
}

func TestParseWebhook_InvalidSignature_ReturnsError(t *testing.T) {
	c := New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	payload, _ := signedPayload(t, "evt1", "checkout.session.completed", `{"id":"cs_1"}`)

	_, err := c.ParseWebhook(payload, "t=1,v1=deadbeef")
	if err == nil {
		t.Fatal("expected an error for an invalid signature")
	}
}

func TestParseWebhook_MissingSignatureHeader_ReturnsError(t *testing.T) {
	c := New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	payload, _ := signedPayload(t, "evt1", "checkout.session.completed", `{"id":"cs_1"}`)

	_, err := c.ParseWebhook(payload, "")
	if err == nil {
		t.Fatal("expected an error for a missing signature header")
	}
}

func TestParseWebhook_CheckoutSessionCompleted_ExtractsSessionAndSubscription(t *testing.T) {
	c := New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	payload, sig := signedPayload(t, "evt1", "checkout.session.completed", `{"id":"cs_123","subscription":"sub_123"}`)

	event, err := c.ParseWebhook(payload, sig)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if event.ID != "evt1" || event.Type != "checkout.session.completed" {
		t.Errorf("event id/type = %q/%q", event.ID, event.Type)
	}
	if event.SessionID != "cs_123" {
		t.Errorf("SessionID = %q, want cs_123", event.SessionID)
	}
	if event.SubscriptionID != "sub_123" {
		t.Errorf("SubscriptionID = %q, want sub_123", event.SubscriptionID)
	}
}

func TestParseWebhook_CheckoutSessionCompleted_NoSubscription_LeavesSubscriptionIDEmpty(t *testing.T) {
	c := New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	payload, sig := signedPayload(t, "evt1", "checkout.session.completed", `{"id":"cs_123"}`)

	event, err := c.ParseWebhook(payload, sig)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if event.SubscriptionID != "" {
		t.Errorf("SubscriptionID = %q, want empty", event.SubscriptionID)
	}
}

func TestParseWebhook_SubscriptionDeleted_ExtractsSubscriptionID(t *testing.T) {
	c := New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	payload, sig := signedPayload(t, "evt2", "customer.subscription.deleted", `{"id":"sub_456"}`)

	event, err := c.ParseWebhook(payload, sig)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if event.SubscriptionID != "sub_456" {
		t.Errorf("SubscriptionID = %q, want sub_456", event.SubscriptionID)
	}
}

func TestParseWebhook_InvoicePaymentFailed_ExtractsSubscriptionAndEmail(t *testing.T) {
	c := New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	payload, sig := signedPayload(t, "evt3", "invoice.payment_failed", `{"subscription":"sub_789","customer_email":"owner@example.com"}`)

	event, err := c.ParseWebhook(payload, sig)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if event.SubscriptionID != "sub_789" {
		t.Errorf("SubscriptionID = %q, want sub_789", event.SubscriptionID)
	}
	if event.CustomerEmail != "owner@example.com" {
		t.Errorf("CustomerEmail = %q, want owner@example.com", event.CustomerEmail)
	}
}

func TestParseWebhook_InvoicePaymentSucceeded_ExtractsSubscriptionID(t *testing.T) {
	c := New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	payload, sig := signedPayload(t, "evt4", "invoice.payment_succeeded", `{"subscription":"sub_abc"}`)

	event, err := c.ParseWebhook(payload, sig)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if event.SubscriptionID != "sub_abc" {
		t.Errorf("SubscriptionID = %q, want sub_abc", event.SubscriptionID)
	}
}

func TestParseWebhook_UnknownEventType_ReturnsEventWithoutFields(t *testing.T) {
	c := New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	payload, sig := signedPayload(t, "evt5", "customer.updated", `{"id":"cus_1"}`)

	event, err := c.ParseWebhook(payload, sig)
	if err != nil {
		t.Fatalf("ParseWebhook: %v", err)
	}
	if event.Type != "customer.updated" {
		t.Errorf("Type = %q, want customer.updated", event.Type)
	}
	if event.SessionID != "" || event.SubscriptionID != "" || event.CustomerEmail != "" {
		t.Errorf("expected no fields extracted for an unhandled event type, got %+v", event)
	}
}
