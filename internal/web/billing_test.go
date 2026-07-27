package web

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stripe/stripe-go/v81/webhook"

	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/payment"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
)

const testWebhookSecret = "whsec_test_secret"

// signedWebhookPayload builds a Stripe event JSON body and a valid
// Stripe-Signature header for it, so StripeWebhook's signature verification
// can be exercised end-to-end without a real Stripe delivery.
func signedWebhookPayload(t *testing.T, eventID, eventType, dataObjectJSON string) (payload []byte, sigHeader string) {
	t.Helper()
	payload = []byte(fmt.Sprintf(`{"id":%q,"object":"event","type":%q,"data":{"object":%s}}`, eventID, eventType, dataObjectJSON))
	ts := time.Now()
	sig := webhook.ComputeSignature(ts, payload, testWebhookSecret)
	sigHeader = fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(sig))
	return payload, sigHeader
}

// newTestBillingHandler wires a Handler whose billing service is backed by a
// sqlmock store, so StripeWebhook can be driven through real signature
// verification and event dispatch without a database or Stripe/Resend
// network calls.
func newTestBillingHandler(t *testing.T) (*Handler, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	pay := payment.New("sk_test", testWebhookSecret, "price_starter", "price_pro")
	mailer := email.New("", "")
	b := service.NewBilling(postgres.NewWithDB(db), pay, mailer, "https://example.launchly.ltd")
	return &Handler{billing: b, render: NewRenderer("launchly.ltd")}, mock
}

func TestStripeWebhook_InvalidSignature_Returns400(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	payload, _ := signedWebhookPayload(t, "evt1", "checkout.session.completed", `{"id":"cs_1"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", "t=1,v1=deadbeef")
	w := httptest.NewRecorder()

	h.StripeWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStripeWebhook_MissingSignatureHeader_Returns400(t *testing.T) {
	h, _ := newTestBillingHandler(t)
	payload, _ := signedWebhookPayload(t, "evt1", "checkout.session.completed", `{"id":"cs_1"}`)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(payload)))
	w := httptest.NewRecorder()

	h.StripeWebhook(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStripeWebhook_UnknownEventType_ClaimsAndReturns200(t *testing.T) {
	h, mock := newTestBillingHandler(t)
	payload, sig := signedWebhookPayload(t, "evt-unknown", "customer.updated", `{"id":"cus_1"}`)

	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt-unknown").
		WillReturnResult(sqlmock.NewResult(1, 1))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	h.StripeWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestStripeWebhook_CheckoutSessionCompleted_NoSessionID_IsNoopReturns200(t *testing.T) {
	h, mock := newTestBillingHandler(t)
	// No "id" on the checkout session object: handleCheckoutCompleted treats
	// an empty SessionID as a no-op, so this exercises the routing to that
	// handler without needing a full site_billing round trip.
	payload, sig := signedWebhookPayload(t, "evt-checkout", "checkout.session.completed", `{}`)

	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt-checkout").
		WillReturnResult(sqlmock.NewResult(1, 1))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	h.StripeWebhook(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d, body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestStripeWebhook_HandlerError_Returns500(t *testing.T) {
	h, mock := newTestBillingHandler(t)
	payload, sig := signedWebhookPayload(t, "evt-fail", "checkout.session.completed", `{"id":"cs_fail"}`)

	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt-fail").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// SetSitePaid failing surfaces immediately as an error from
	// handleCheckoutCompleted, with no further DB or mailer calls.
	mock.ExpectExec("UPDATE site_billing SET payment_status = 'paid'").
		WithArgs(sqlmock.AnyArg(), "", "cs_fail").
		WillReturnError(sqlmock.ErrCancelled)
	// The claim taken above must be released so Stripe's automatic retry
	// isn't permanently skipped.
	mock.ExpectExec("DELETE FROM stripe_events").
		WithArgs("evt-fail").
		WillReturnResult(sqlmock.NewResult(0, 1))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/stripe", strings.NewReader(string(payload)))
	req.Header.Set("Stripe-Signature", sig)
	w := httptest.NewRecorder()

	h.StripeWebhook(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d, body=%s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
