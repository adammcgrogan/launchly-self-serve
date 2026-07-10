package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// Cron runs background reminders: trial-ending emails and scheduled
// analytics digests. Trial reminders link straight to the dashboard upgrade
// button — there is no admin-sent payment link to wait on.
type Cron struct {
	store     *postgres.Store
	mailer    *email.Client
	analytics *Analytics
	baseURL   string
}

func NewCron(store *postgres.Store, mailer *email.Client, analytics *Analytics, baseURL string) *Cron {
	return &Cron{store: store, mailer: mailer, analytics: analytics, baseURL: baseURL}
}

// Start launches the background tickers. Call once at server startup.
func (c *Cron) Start() {
	go c.runEvery(time.Hour, c.sendDueTrialReminders)
	go c.runEvery(time.Hour, c.sendDueAnalyticsDigests)
}

func (c *Cron) runEvery(interval time.Duration, fn func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		fn()
	}
}

func (c *Cron) sendDueTrialReminders() {
	ctx := context.Background()
	for _, kind := range []string{"first", "final"} {
		ids, err := postgres.GetSiteIDsDueForTrialReminder(ctx, c.store.DB(), kind)
		if err != nil {
			slog.Error("trial cron: list sites", "kind", kind, "error", err)
			continue
		}
		for _, id := range ids {
			site, err := postgres.GetSiteByID(ctx, c.store.DB(), id)
			if err != nil || site == nil {
				continue
			}
			contact, err := postgres.GetSiteContact(ctx, c.store.DB(), id)
			if err != nil || contact == nil || contact.Email == "" {
				continue
			}
			daysLeft := 3
			if kind == "final" {
				daysLeft = 1
			}
			dashboardURL := fmt.Sprintf("%s/dashboard/sites/%d", c.baseURL, id)
			if err := c.mailer.SendTrialWarning(contact.Email, site.BusinessName, dashboardURL, daysLeft); err != nil {
				slog.Error("trial cron: send reminder", "slug", site.Slug, "kind", kind, "error", err)
				continue
			}
			if err := postgres.MarkTrialReminderSent(ctx, c.store.DB(), id, kind); err != nil {
				slog.Error("trial cron: mark sent", "slug", site.Slug, "kind", kind, "error", err)
			} else {
				slog.Info("trial reminder sent", "slug", site.Slug, "kind", kind)
			}
		}
	}
}

func (c *Cron) sendDueAnalyticsDigests() {
	ctx := context.Background()
	ids, err := postgres.GetSiteIDsDueForAnalytics(ctx, c.store.DB())
	if err != nil {
		slog.Error("analytics cron: list sites", "error", err)
		return
	}
	for _, id := range ids {
		if err := c.SendAnalyticsReport(ctx, id); err != nil {
			slog.Error("analytics cron: send report", "site_id", id, "error", err)
		}
	}
}

// SendAnalyticsReport builds stats and emails the analytics digest for a
// site. Used by both the cron ticker and the dashboard's "send now" action.
func (c *Cron) SendAnalyticsReport(ctx context.Context, siteID int) error {
	site, err := postgres.GetSiteByID(ctx, c.store.DB(), siteID)
	if err != nil || site == nil {
		return err
	}
	contact, err := postgres.GetSiteContact(ctx, c.store.DB(), siteID)
	if err != nil || contact == nil || contact.Email == "" {
		return err
	}
	settings, err := postgres.GetSiteAnalyticsSettings(ctx, c.store.DB(), siteID)
	if err != nil {
		return err
	}
	days := 7
	if settings.AnalyticsFrequency == "monthly" {
		days = 30
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	stats, err := c.analytics.GetSiteStats(ctx, siteID, since)
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}
	siteURL := fmt.Sprintf("%s/dashboard/sites/%d", c.baseURL, siteID)
	freq := settings.AnalyticsFrequency
	if freq == "" || freq == "off" {
		freq = "weekly"
	}
	if err := c.mailer.SendAnalyticsDigest(contact.Email, site.BusinessName, freq, stats, siteURL); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return postgres.UpdateAnalyticsLastSent(ctx, c.store.DB(), siteID)
}
