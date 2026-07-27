package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func TestGetSitesDueForTrialReminder_UnknownKind_ReturnsErrorWithoutQuerying(t *testing.T) {
	db, mock := newMockDB(t)

	_, err := GetSitesDueForTrialReminder(context.Background(), db, "bogus")
	if err == nil {
		t.Fatal("expected an error for an unknown reminder kind")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestGetSitesDueForTrialReminder_KindSelectsCorrectColumnAndWindow checks
// that each reminder kind checks its own sent-at column (so "first" and
// "final" are independent, per-stage flags rather than a shared one — see
// #198) and its own due window, and that both kinds still exclude sites
// that are already paid or cancelled.
func TestGetSitesDueForTrialReminder_KindSelectsCorrectColumnAndWindow(t *testing.T) {
	tests := []struct {
		kind       string
		wantColumn string
		wantWindow string
	}{
		{"first", "trial_reminder_sent_at", "INTERVAL '3 days'"},
		{"final", "trial_final_reminder_sent_at", "INTERVAL '1 day'"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			db, mock := newMockDB(t)
			trialEndsAt := time.Now().UTC().Add(20 * time.Hour)

			mock.ExpectQuery(tt.wantColumn + " IS NULL").
				WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "business_name", "trial_ends_at", "notify_email"}).
					AddRow(1, "acme", "Acme Co", trialEndsAt, "owner@acme.test"))

			due, err := GetSitesDueForTrialReminder(context.Background(), db, tt.kind)
			if err != nil {
				t.Fatalf("GetSitesDueForTrialReminder: %v", err)
			}
			if len(due) != 1 || due[0].SiteID != 1 || due[0].Slug != "acme" || due[0].NotifyEmail != "owner@acme.test" {
				t.Fatalf("due = %+v", due)
			}
			if !due[0].TrialEndsAt.Equal(trialEndsAt) {
				t.Errorf("TrialEndsAt = %v, want %v", due[0].TrialEndsAt, trialEndsAt)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestGetSitesDueForTrialReminder_ExcludesPaidAndCancelled(t *testing.T) {
	db, mock := newMockDB(t)

	// Assert the query text itself carries the payment_status exclusion —
	// a paid or cancelled site must never be selected as due, regardless of
	// trial_ends_at.
	mock.ExpectQuery(`payment_status NOT IN \('paid', 'cancelled'\)`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "business_name", "trial_ends_at", "notify_email"}))

	if _, err := GetSitesDueForTrialReminder(context.Background(), db, "first"); err != nil {
		t.Fatalf("GetSitesDueForTrialReminder: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestGetSitesDueForTrialPause_FiltersLiveAndUnpaid checks that the pause
// query's WHERE clause excludes paid sites and non-live sites — a paid site
// or one that's already draft/paused must never come back as due for pause.
func TestGetSitesDueForTrialPause_FiltersLiveAndUnpaid(t *testing.T) {
	db, mock := newMockDB(t)
	cutoff := time.Now().UTC()

	mock.ExpectQuery(`payment_status != 'paid'[\s\S]*s\.status = 'live'`).
		WithArgs(cutoff).
		WillReturnRows(sqlmock.NewRows([]string{"id", "slug", "business_name", "notify_email"}).
			AddRow(2, "beta", "Beta Co", "owner@beta.test"))

	due, err := GetSitesDueForTrialPause(context.Background(), db, cutoff)
	if err != nil {
		t.Fatalf("GetSitesDueForTrialPause: %v", err)
	}
	if len(due) != 1 || due[0].SiteID != 2 || due[0].Slug != "beta" || due[0].NotifyEmail != "owner@beta.test" {
		t.Fatalf("due = %+v", due)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMarkTrialReminderSent_UnknownKind_ReturnsErrorWithoutQuerying(t *testing.T) {
	db, mock := newMockDB(t)

	if err := MarkTrialReminderSent(context.Background(), db, 1, "bogus"); err == nil {
		t.Fatal("expected an error for an unknown reminder kind")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestMarkTrialReminderSent_UpdatesColumnPerKind(t *testing.T) {
	tests := []struct {
		kind       string
		wantColumn string
	}{
		{"first", "trial_reminder_sent_at"},
		{"final", "trial_final_reminder_sent_at"},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			db, mock := newMockDB(t)

			mock.ExpectExec("UPDATE site_billing SET " + tt.wantColumn).
				WithArgs(5).
				WillReturnResult(sqlmock.NewResult(0, 1))

			if err := MarkTrialReminderSent(context.Background(), db, 5, tt.kind); err != nil {
				t.Fatalf("MarkTrialReminderSent: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestSetSitesPaused_EmptyIDs_NoQueryIssued(t *testing.T) {
	db, mock := newMockDB(t)

	if err := SetSitesPaused(context.Background(), db, nil); err != nil {
		t.Fatalf("SetSitesPaused: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSetSitesPaused_NonEmptyIDs_IssuesUpdate(t *testing.T) {
	db, mock := newMockDB(t)

	mock.ExpectExec("UPDATE sites SET status = 'paused'").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))

	if err := SetSitesPaused(context.Background(), db, []int{1, 2}); err != nil {
		t.Fatalf("SetSitesPaused: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
