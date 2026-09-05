package service

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

// fakeCronMailer records every call instead of hitting Resend, so tests can
// assert exactly which reminder/pause emails a sweep triggered — and how
// many times.
type fakeCronMailer struct {
	trialWarnings []string // "to|businessName|dashboardURL|daysLeft"
	sitePaused    []string // "to|businessName|dashboardURL"
	earlyReports  []string // "to|businessName|dashboardURL|totalViews"
	weekReports   []string // "to|businessName|dashboardURL|totalViews|daysLeft"
	sendErr       error    // if set, every Send* returns this instead of recording
}

func (m *fakeCronMailer) SendTrialWarning(to, businessName, dashboardURL string, daysLeft int) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.trialWarnings = append(m.trialWarnings, fmt.Sprintf("%s|%s|%s|%d", to, businessName, dashboardURL, daysLeft))
	return nil
}

func (m *fakeCronMailer) SendSitePaused(to, businessName, dashboardURL string) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sitePaused = append(m.sitePaused, to+"|"+businessName+"|"+dashboardURL)
	return nil
}

func (m *fakeCronMailer) SendTrialEarlyReport(to, businessName, dashboardURL string, stats *domain.SiteStats) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.earlyReports = append(m.earlyReports, fmt.Sprintf("%s|%s|%s|%d", to, businessName, dashboardURL, stats.TotalViews))
	return nil
}

func (m *fakeCronMailer) SendTrialWeekReport(to, businessName, dashboardURL string, stats *domain.SiteStats, daysLeft int) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.weekReports = append(m.weekReports, fmt.Sprintf("%s|%s|%s|%d|%d", to, businessName, dashboardURL, stats.TotalViews, daysLeft))
	return nil
}

func (m *fakeCronMailer) SendAnalyticsDigest(to, businessName string, stats *domain.SiteStats, siteURL string) error {
	return nil
}

func (m *fakeCronMailer) SendDunningReminder(to, businessName, dashboardURL string, daysPastDue int) error {
	return nil
}

func (m *fakeCronMailer) SendFinalPaymentWarning(to, businessName, dashboardURL string) error {
	return nil
}

// newTestCron wires a Cron against a sqlmock-backed store and a fake mailer,
// so sendDueTrialReminders/pauseDueSites can be exercised without a real
// Postgres instance or network calls to Resend.
func newTestCron(t *testing.T) (*Cron, sqlmock.Sqlmock, *fakeCronMailer) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	mailer := &fakeCronMailer{}
	store := postgres.NewWithDB(db)
	c := &Cron{
		store:     store,
		mailer:    mailer,
		analytics: &Analytics{store: store},
		baseURL:   "https://example.launchly.ltd",
	}
	return c, mock, mailer
}

// sqlmock runs in ordered mode, so the advisory-lock acquire/release pair
// withAdvisoryLock takes around every sweep has to be split in two: queue
// the acquire before the sweep's own query/exec expectations, then the
// release after them.
func expectAdvisoryLockAcquire(mock sqlmock.Sqlmock, key int64) {
	mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs(key).
		WillReturnRows(sqlmock.NewRows([]string{"pg_try_advisory_lock"}).AddRow(true))
}

func expectAdvisoryLockRelease(mock sqlmock.Sqlmock, key int64) {
	mock.ExpectExec("SELECT pg_advisory_unlock").WithArgs(key).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func trialReminderColumns() []string {
	return []string{"owner_user_id", "id", "slug", "business_name", "trial_ends_at", "timezone", "notify_email"}
}

// trialReminderRows builds a due row for one account. Trials are per-account
// (#338), so the sweep keys everything — dedupe, mark-sent — on the owner.
func trialReminderRows(ownerID uuid.UUID, siteID int, slug, businessName string, trialEndsAt time.Time, notifyEmail string) *sqlmock.Rows {
	return sqlmock.NewRows(trialReminderColumns()).
		AddRow(ownerID, siteID, slug, businessName, trialEndsAt, "Europe/London", notifyEmail)
}

func emptyTrialReminderRows() *sqlmock.Rows {
	return sqlmock.NewRows(trialReminderColumns())
}

// expectNoReportsDue queues the two value-report queries (#330) returning
// nothing, so tests about the billing warnings don't have to care that the
// sweep now runs four kinds rather than two. Must be queued after the
// "final"/"first" expectations — sqlmock is ordered, and the sweep runs the
// reports last.
func expectNoReportsDue(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("ab.trial_report_sent_at IS NULL").WillReturnRows(emptyTrialReminderRows())
	mock.ExpectQuery("trial_early_report_sent_at IS NULL").WillReturnRows(emptyTrialReminderRows())
}

// TestSendDueTrialReminders_DueForBothKindsInOneSweep_SendsOnlyFinal covers
// #198: a cron gap wide enough that a site matches both the "first" and
// "final" due-queries in the same sweep must only get the higher-priority
// "final" reminder, not both back-to-back.
func TestSendDueTrialReminders_DueForBothKindsInOneSweep_SendsOnlyFinal(t *testing.T) {
	c, mock, mailer := newTestCron(t)
	ownerID := uuid.New()
	trialEndsAt := time.Now().UTC().Add(20 * time.Hour)

	expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
	// "final" is checked first.
	mock.ExpectQuery("trial_final_reminder_sent_at IS NULL").
		WillReturnRows(trialReminderRows(ownerID, 42, "acme", "Acme Co", trialEndsAt, "owner@acme.test"))
	mock.ExpectExec("UPDATE account_billing SET trial_final_reminder_sent_at").
		WithArgs(ownerID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// The same account is also (falsely) due for "first" in this sweep; it
	// must be skipped since it already got a reminder this pass.
	mock.ExpectQuery("ab.trial_reminder_sent_at IS NULL").
		WillReturnRows(trialReminderRows(ownerID, 42, "acme", "Acme Co", trialEndsAt, "owner@acme.test"))
	expectNoReportsDue(mock)
	expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

	c.sendDueTrialReminders()

	if len(mailer.trialWarnings) != 1 {
		t.Fatalf("expected exactly one trial warning sent, got %v", mailer.trialWarnings)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestSendDueTrialReminders_DaysLeftFromTrialEndsAt covers #148: daysLeft
// must be computed fresh from trial_ends_at each sweep, not assumed from
// which due-query matched, and rounds up rather than down or to zero.
func TestSendDueTrialReminders_DaysLeftFromTrialEndsAt(t *testing.T) {
	tests := []struct {
		name         string
		offset       time.Duration
		expectedDays int
	}{
		{"just under a day left rounds up to 1", 6 * time.Hour, 1},
		{"just over a day left rounds up to 2", 30 * time.Hour, 2},
		{"several days out rounds up", 100 * time.Hour, 5},
		{"already past due clamps to 1", -2 * time.Hour, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mock, mailer := newTestCron(t)
			ownerID := uuid.New()
			trialEndsAt := time.Now().UTC().Add(tt.offset)

			expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
			mock.ExpectQuery("trial_final_reminder_sent_at IS NULL").
				WillReturnRows(emptyTrialReminderRows())
			mock.ExpectQuery("ab.trial_reminder_sent_at IS NULL").
				WillReturnRows(trialReminderRows(ownerID, 1, "site1", "Site One", trialEndsAt, "owner@site1.test"))
			mock.ExpectExec("UPDATE account_billing SET trial_reminder_sent_at").
				WithArgs(ownerID).
				WillReturnResult(sqlmock.NewResult(0, 1))
			expectNoReportsDue(mock)
			expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

			c.sendDueTrialReminders()

			if len(mailer.trialWarnings) != 1 {
				t.Fatalf("expected exactly one trial warning, got %v", mailer.trialWarnings)
			}
			want := fmt.Sprintf("owner@site1.test|Site One|https://example.launchly.ltd/dashboard/sites/site1|%d", tt.expectedDays)
			if got := mailer.trialWarnings[0]; got != want {
				t.Errorf("trial warning = %q, want %q", got, want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

func TestSendDueTrialReminders_NoNotifyEmail_SkipsSiteEntirely(t *testing.T) {
	c, mock, mailer := newTestCron(t)
	trialEndsAt := time.Now().UTC().Add(20 * time.Hour)

	expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
	mock.ExpectQuery("trial_final_reminder_sent_at IS NULL").
		WillReturnRows(trialReminderRows(uuid.New(), 7, "no-email", "No Email Co", trialEndsAt, ""))
	mock.ExpectQuery("ab.trial_reminder_sent_at IS NULL").
		WillReturnRows(emptyTrialReminderRows())
	expectNoReportsDue(mock)
	expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

	c.sendDueTrialReminders()

	if len(mailer.trialWarnings) != 0 {
		t.Errorf("expected no trial warnings for a site with no notify email, got %v", mailer.trialWarnings)
	}
	// No MarkTrialReminderSent exec is expected above, so ExpectationsWereMet
	// also proves the site was never marked as reminded.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSendDueTrialReminders_SendFailure_DoesNotMarkSent(t *testing.T) {
	c, mock, mailer := newTestCron(t)
	mailer.sendErr = sql.ErrConnDone // any non-nil error from the mailer
	trialEndsAt := time.Now().UTC().Add(20 * time.Hour)

	expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
	mock.ExpectQuery("trial_final_reminder_sent_at IS NULL").
		WillReturnRows(trialReminderRows(uuid.New(), 9, "fails-to-send", "Fails Co", trialEndsAt, "owner@fails.test"))
	mock.ExpectQuery("ab.trial_reminder_sent_at IS NULL").
		WillReturnRows(emptyTrialReminderRows())
	// No UPDATE ... trial_final_reminder_sent_at exec is expected: since the
	// send failed, the site must not be marked as reminded (or it would
	// silently never get retried).
	expectNoReportsDue(mock)
	expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

	c.sendDueTrialReminders()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (marked sent despite send failure?): %v", err)
	}
}

func TestPauseDueSites_PausesAndNotifiesOnlySitesWithNotifyEmail(t *testing.T) {
	c, mock, mailer := newTestCron(t)
	ownerA, ownerB := uuid.New(), uuid.New()

	expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
	mock.ExpectQuery("FROM account_billing ab").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id", "id", "slug", "business_name", "notify_email"}).
			AddRow(ownerA, 11, "acme", "Acme Co", "owner@acme.test").
			AddRow(ownerB, 12, "beta", "Beta Co", ""))
	mock.ExpectExec("UPDATE sites SET status = 'paused'").
		WithArgs(sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 2))
	expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

	c.pauseDueSites()

	if len(mailer.sitePaused) != 1 || mailer.sitePaused[0] != "owner@acme.test|Acme Co|https://example.launchly.ltd/dashboard/sites/acme" {
		t.Errorf("sitePaused = %v", mailer.sitePaused)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestPauseDueSites_NoDueSites_NoPauseOrNotify(t *testing.T) {
	c, mock, mailer := newTestCron(t)

	expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
	mock.ExpectQuery("FROM account_billing ab").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id", "id", "slug", "business_name", "notify_email"}))
	// No UPDATE sites ... 'paused' exec is expected: with zero due sites,
	// pauseDueSites must return early rather than issue a no-op pause.
	expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

	c.pauseDueSites()

	if len(mailer.sitePaused) != 0 {
		t.Errorf("expected no paused-site emails, got %v", mailer.sitePaused)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (paused despite no due sites?): %v", err)
	}
}

func TestPauseDueSites_PauseExecFails_NoNotificationsSent(t *testing.T) {
	c, mock, mailer := newTestCron(t)

	expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
	mock.ExpectQuery("FROM account_billing ab").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id", "id", "slug", "business_name", "notify_email"}).
			AddRow(uuid.New(), 11, "acme", "Acme Co", "owner@acme.test"))
	mock.ExpectExec("UPDATE sites SET status = 'paused'").
		WithArgs(sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)
	expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

	c.pauseDueSites()

	if len(mailer.sitePaused) != 0 {
		t.Errorf("expected no paused-site emails when the pause update itself failed, got %v", mailer.sitePaused)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// siteStatsRows is a minimal GetSiteStats result — enough for the trial
// reports to render, without restating the whole analytics query here.
func siteStatsRows(totalViews, uniqueVisitors int) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"total_views", "unique_visitors", "prev_total_views", "prev_unique_visitors",
		"referrers", "pages", "days", "events", "prev_leads",
	}).AddRow(totalViews, uniqueVisitors, 0, 0, []byte("[]"), []byte("[]"), []byte("[]"), []byte("[]"), 0)
}

// TestSendDueTrialReminders_SendsValueReports covers #330: the day-3 and
// day-5 reports are what stop every trial email being about billing, so a
// site due for one must get it with its own traffic numbers attached.
func TestSendDueTrialReminders_SendsValueReports(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		markCol  string
		earlyLen int
		weekLen  int
	}{
		{"day 5 week report", "ab.trial_report_sent_at IS NULL", "UPDATE account_billing SET trial_report_sent_at", 0, 1},
		{"day 3 early report", "trial_early_report_sent_at IS NULL", "UPDATE account_billing SET trial_early_report_sent_at", 1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, mock, mailer := newTestCron(t)
			ownerID := uuid.New()
			trialEndsAt := time.Now().UTC().Add(48 * time.Hour)

			expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
			mock.ExpectQuery("trial_final_reminder_sent_at IS NULL").WillReturnRows(emptyTrialReminderRows())
			mock.ExpectQuery("ab.trial_reminder_sent_at IS NULL").WillReturnRows(emptyTrialReminderRows())
			if tt.weekLen == 0 {
				mock.ExpectQuery("ab.trial_report_sent_at IS NULL").WillReturnRows(emptyTrialReminderRows())
			}
			mock.ExpectQuery(tt.query).
				WillReturnRows(trialReminderRows(ownerID, 5, "acme", "Acme Co", trialEndsAt, "owner@acme.test"))
			mock.ExpectQuery("FROM page_views").WillReturnRows(siteStatsRows(12, 9))
			mock.ExpectExec(tt.markCol).WithArgs(ownerID).WillReturnResult(sqlmock.NewResult(0, 1))
			if tt.earlyLen == 0 {
				mock.ExpectQuery("trial_early_report_sent_at IS NULL").WillReturnRows(emptyTrialReminderRows())
			}
			expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

			c.sendDueTrialReminders()

			if len(mailer.earlyReports) != tt.earlyLen || len(mailer.weekReports) != tt.weekLen {
				t.Fatalf("earlyReports=%v weekReports=%v", mailer.earlyReports, mailer.weekReports)
			}
			// No trial warning: a report must not double as an upgrade nudge.
			if len(mailer.trialWarnings) != 0 {
				t.Errorf("expected no trial warnings, got %v", mailer.trialWarnings)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("unmet expectations: %v", err)
			}
		})
	}
}

// TestSendDueTrialReminders_ReportYieldsToFinalWarning covers the priority
// ordering: a lagging sweep where a site matches both the final warning and
// a value report must send only the warning — the deadline is the more
// urgent thing to say, and sentThisSweep stops the pair going out together.
func TestSendDueTrialReminders_ReportYieldsToFinalWarning(t *testing.T) {
	c, mock, mailer := newTestCron(t)
	ownerID := uuid.New()
	trialEndsAt := time.Now().UTC().Add(20 * time.Hour)

	expectAdvisoryLockAcquire(mock, advisoryLockTrialCron)
	mock.ExpectQuery("trial_final_reminder_sent_at IS NULL").
		WillReturnRows(trialReminderRows(ownerID, 42, "acme", "Acme Co", trialEndsAt, "owner@acme.test"))
	mock.ExpectExec("UPDATE account_billing SET trial_final_reminder_sent_at").
		WithArgs(ownerID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("ab.trial_reminder_sent_at IS NULL").WillReturnRows(emptyTrialReminderRows())
	// The same account is also due for the day-5 report; it must be skipped,
	// so no stats query and no mark-sent exec follow.
	mock.ExpectQuery("ab.trial_report_sent_at IS NULL").
		WillReturnRows(trialReminderRows(ownerID, 42, "acme", "Acme Co", trialEndsAt, "owner@acme.test"))
	mock.ExpectQuery("trial_early_report_sent_at IS NULL").WillReturnRows(emptyTrialReminderRows())
	expectAdvisoryLockRelease(mock, advisoryLockTrialCron)

	c.sendDueTrialReminders()

	if len(mailer.trialWarnings) != 1 {
		t.Fatalf("expected exactly one trial warning, got %v", mailer.trialWarnings)
	}
	if len(mailer.weekReports) != 0 {
		t.Errorf("expected no report alongside the final warning, got %v", mailer.weekReports)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
