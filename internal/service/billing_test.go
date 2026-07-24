package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/payment"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

// fakeMailer records every call instead of hitting Resend, so tests can
// assert which notifications a webhook handler triggered.
type fakeMailer struct {
	paymentConfirmations []string // "to|businessName|plan"
	cancellations        []string // "to|businessName"
	paymentFailed        []string // "to|businessName"
	adminAlerts          []string // "subject|message"
}

func (m *fakeMailer) SendPaymentConfirmation(to, businessName string, plan domain.Plan) error {
	m.paymentConfirmations = append(m.paymentConfirmations, to+"|"+businessName+"|"+string(plan))
	return nil
}

func (m *fakeMailer) SendCancellationConfirmation(to, businessName string) error {
	m.cancellations = append(m.cancellations, to+"|"+businessName)
	return nil
}

func (m *fakeMailer) SendPaymentFailed(to, businessName string) error {
	m.paymentFailed = append(m.paymentFailed, to+"|"+businessName)
	return nil
}

func (m *fakeMailer) SendAdminAlert(to, subject, message string) error {
	m.adminAlerts = append(m.adminAlerts, subject+"|"+message)
	return nil
}

// newTestBilling wires a Billing against a sqlmock-backed store and a fake
// mailer, so the webhook dispatch logic in billing.go can be exercised
// without a real Postgres instance or network calls to Resend/Stripe.
func newTestBilling(t *testing.T) (*Billing, sqlmock.Sqlmock, *fakeMailer) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mailer := &fakeMailer{}
	b := &Billing{
		store:   postgres.NewWithDB(db),
		pay:     &payment.Client{},
		mailer:  mailer,
		baseURL: "https://example.launchly.ltd",
	}
	return b, mock, mailer
}

const siteColumnCount = 30

// siteRows returns a single-row sqlmock result set matching the exact
// column order sites.go's scanSite expects (see siteColumns in
// internal/repository/postgres/sites.go).
func siteRows(id int, ownerID uuid.UUID, businessName string, status domain.SiteStatus) *sqlmock.Rows {
	cols := []string{
		"id", "owner_user_id", "slug", "business_name", "tagline", "about", "logo_url", "cta_text",
		"template_id", "form_type", "palette", "heading_font", "brand_color", "status", "created_at", "published_at", "updated_at", "slug_changed_at",
		"custom_domain", "custom_domain_status", "custom_domain_cf_id", "custom_domain_added_at", "timezone",
		"meta_title", "meta_description", "og_image_url", "video_url", "is_demo", "thank_you_message", "redirect_url",
	}
	if len(cols) != siteColumnCount {
		panic("siteRows column count mismatch")
	}
	now := time.Now().UTC()
	return sqlmock.NewRows(cols).AddRow(
		id, ownerID, "test-site", businessName, "tagline", "about", "", "Contact us",
		"template-a", domain.FormType("standard"), "classic", "sans", "#4F46E5", string(status), now, nil, now, nil,
		nil, "none", nil, nil, "Europe/London",
		"", "", "", "", false, "", "",
	)
}

func contactRows(email string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"phone", "email", "address", "location", "map_url", "map_embed_url"}).
		AddRow("", email, "", "", "", "")
}

func billingRows(siteID int, plan domain.Plan) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"site_id", "plan", "payment_status", "stripe_customer_id", "stripe_session_id", "stripe_subscription_id",
		"paid_at", "trial_ends_at", "trial_reminder_sent_at", "trial_final_reminder_sent_at",
		"payment_failed_at", "dunning_reminder_1_sent_at", "dunning_reminder_2_sent_at", "dunning_final_warning_sent_at",
	}).AddRow(siteID, string(plan), "pending", "", "sess1", "", nil, nil, nil, nil, nil, nil, nil, nil)
}

func TestHandleWebhookEvent_CheckoutCompleted_ReactivatesPausedSiteAndNotifies(t *testing.T) {
	b, mock, mailer := newTestBilling(t)
	ownerID := uuid.New()

	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("UPDATE site_billing SET payment_status = 'paid'").
		WithArgs(sqlmock.AnyArg(), "sub1", "sess1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("FROM site_billing WHERE stripe_session_id").
		WithArgs("sess1").
		WillReturnRows(billingRows(42, domain.PlanStarter))
	mock.ExpectQuery("FROM sites WHERE id").
		WithArgs(42).
		WillReturnRows(siteRows(42, ownerID, "Acme Co", domain.SiteStatusPaused))
	mock.ExpectExec("UPDATE sites SET status = 'live'").
		WithArgs(42).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM site_contact WHERE site_id").
		WithArgs(42).
		WillReturnRows(contactRows("owner@acme.test"))
	mock.ExpectQuery("FROM profiles").
		WithArgs(ownerID).
		WillReturnError(sql.ErrNoRows)

	event := &payment.WebhookEvent{ID: "evt1", Type: "checkout.session.completed", SessionID: "sess1", SubscriptionID: "sub1"}
	if err := b.HandleWebhookEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleWebhookEvent: %v", err)
	}

	if len(mailer.paymentConfirmations) != 1 || mailer.paymentConfirmations[0] != "owner@acme.test|Acme Co|starter" {
		t.Errorf("payment confirmations = %v", mailer.paymentConfirmations)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHandleWebhookEvent_CheckoutCompleted_RepeatDelivery_NoDoubleNotify(t *testing.T) {
	b, mock, mailer := newTestBilling(t)

	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt1").
		WillReturnResult(sqlmock.NewResult(1, 1))
	// SetSitePaid affects 0 rows: the site was already marked paid by an
	// earlier delivery of this same event.
	mock.ExpectExec("UPDATE site_billing SET payment_status = 'paid'").
		WithArgs(sqlmock.AnyArg(), "sub1", "sess1").
		WillReturnResult(sqlmock.NewResult(0, 0))

	event := &payment.WebhookEvent{ID: "evt1", Type: "checkout.session.completed", SessionID: "sess1", SubscriptionID: "sub1"}
	if err := b.HandleWebhookEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleWebhookEvent: %v", err)
	}

	if len(mailer.paymentConfirmations) != 0 {
		t.Errorf("expected no payment confirmation on repeat delivery, got %v", mailer.paymentConfirmations)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHandleWebhookEvent_DuplicateDeliveryAlreadyProcessed_IsNoop(t *testing.T) {
	b, mock, mailer := newTestBilling(t)

	// The INSERT ... ON CONFLICT DO NOTHING affects 0 rows: another
	// delivery of this event already claimed and processed it, so nothing
	// else should run.
	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt-dup").
		WillReturnResult(sqlmock.NewResult(0, 0))

	event := &payment.WebhookEvent{ID: "evt-dup", Type: "customer.subscription.deleted", SubscriptionID: "sub1"}
	if err := b.HandleWebhookEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleWebhookEvent: %v", err)
	}

	if len(mailer.cancellations) != 0 || len(mailer.adminAlerts) != 0 {
		t.Errorf("expected no notifications for an already-processed event, got cancellations=%v alerts=%v",
			mailer.cancellations, mailer.adminAlerts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHandleWebhookEvent_SubscriptionDeleted_PausesLiveSiteAndNotifies(t *testing.T) {
	b, mock, mailer := newTestBilling(t)
	ownerID := uuid.New()

	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt2").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("FROM site_billing WHERE stripe_subscription_id").
		WithArgs("sub1").
		WillReturnRows(billingRows(42, domain.PlanPro))
	mock.ExpectExec("UPDATE site_billing SET payment_status = 'cancelled'").
		WithArgs("sub1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM sites WHERE id").
		WithArgs(42).
		WillReturnRows(siteRows(42, ownerID, "Acme Co", domain.SiteStatusLive))
	mock.ExpectQuery("FROM site_contact WHERE site_id").
		WithArgs(42).
		WillReturnRows(contactRows("owner@acme.test"))
	mock.ExpectExec("UPDATE sites SET status = 'paused'").
		WithArgs(42).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM profiles").
		WithArgs(ownerID).
		WillReturnError(sql.ErrNoRows)

	event := &payment.WebhookEvent{ID: "evt2", Type: "customer.subscription.deleted", SubscriptionID: "sub1"}
	if err := b.HandleWebhookEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleWebhookEvent: %v", err)
	}

	if len(mailer.cancellations) != 1 || mailer.cancellations[0] != "owner@acme.test|Acme Co" {
		t.Errorf("cancellations = %v", mailer.cancellations)
	}
	if len(mailer.adminAlerts) != 1 {
		t.Errorf("expected one admin alert, got %v", mailer.adminAlerts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHandleWebhookEvent_PaymentFailed_NotifiesOwnerAndAdmin(t *testing.T) {
	b, mock, mailer := newTestBilling(t)
	ownerID := uuid.New()

	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt3").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("FROM site_billing WHERE stripe_subscription_id").
		WithArgs("sub1").
		WillReturnRows(billingRows(42, domain.PlanPro))
	mock.ExpectExec("UPDATE site_billing SET payment_status = 'past_due'").
		WithArgs("sub1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM sites WHERE id").
		WithArgs(42).
		WillReturnRows(siteRows(42, ownerID, "Acme Co", domain.SiteStatusLive))
	mock.ExpectQuery("FROM site_contact WHERE site_id").
		WithArgs(42).
		WillReturnRows(contactRows("owner@acme.test"))
	mock.ExpectQuery("FROM profiles").
		WithArgs(ownerID).
		WillReturnError(sql.ErrNoRows)

	event := &payment.WebhookEvent{ID: "evt3", Type: "invoice.payment_failed", SubscriptionID: "sub1"}
	if err := b.HandleWebhookEvent(context.Background(), event); err != nil {
		t.Fatalf("HandleWebhookEvent: %v", err)
	}

	if len(mailer.paymentFailed) != 1 || mailer.paymentFailed[0] != "owner@acme.test|Acme Co" {
		t.Errorf("paymentFailed = %v", mailer.paymentFailed)
	}
	if len(mailer.adminAlerts) != 1 {
		t.Errorf("expected one admin alert, got %v", mailer.adminAlerts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestHandleWebhookEvent_ProcessingFailure_ReleasesClaimForRetry(t *testing.T) {
	b, mock, _ := newTestBilling(t)

	mock.ExpectExec("INSERT INTO stripe_events").
		WithArgs("evt4").
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("FROM site_billing WHERE stripe_subscription_id").
		WithArgs("sub1").
		WillReturnRows(billingRows(42, domain.PlanPro))
	mock.ExpectExec("UPDATE site_billing SET payment_status = 'cancelled'").
		WithArgs("sub1").
		WillReturnError(sql.ErrConnDone)
	// The claim taken above must be released so Stripe's automatic retry
	// isn't permanently skipped.
	mock.ExpectExec("DELETE FROM stripe_events").
		WithArgs("evt4").
		WillReturnResult(sqlmock.NewResult(0, 1))

	event := &payment.WebhookEvent{ID: "evt4", Type: "customer.subscription.deleted", SubscriptionID: "sub1"}
	if err := b.HandleWebhookEvent(context.Background(), event); err == nil {
		t.Fatal("expected an error to be returned so Stripe retries")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (claim not released?): %v", err)
	}
}

func TestSetSitePlanAfterStripeChange_SucceedsOnRetry(t *testing.T) {
	b, mock, mailer := newTestBilling(t)

	mock.ExpectExec("UPDATE site_billing SET plan").
		WithArgs("pro", 42).
		WillReturnError(sql.ErrConnDone)
	mock.ExpectExec("UPDATE site_billing SET plan").
		WithArgs("pro", 42).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := b.setSitePlanAfterStripeChange(context.Background(), 42, domain.PlanPro, "sub1"); err != nil {
		t.Fatalf("setSitePlanAfterStripeChange: %v", err)
	}
	if len(mailer.adminAlerts) != 0 {
		t.Errorf("expected no alert when a retry succeeds, got %v", mailer.adminAlerts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSetSitePlanAfterStripeChange_AlertsAfterExhaustingRetries(t *testing.T) {
	b, mock, mailer := newTestBilling(t)

	for i := 0; i < 3; i++ {
		mock.ExpectExec("UPDATE site_billing SET plan").
			WithArgs("pro", 42).
			WillReturnError(sql.ErrConnDone)
	}

	err := b.setSitePlanAfterStripeChange(context.Background(), 42, domain.PlanPro, "sub1")
	if err == nil {
		t.Fatal("expected an error after exhausting all retries")
	}
	if len(mailer.adminAlerts) != 1 {
		t.Fatalf("expected exactly one admin alert after exhausting retries, got %v", mailer.adminAlerts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
