package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// trialGracePeriod is how long a site stays live after its trial ends
// before being paused. Trials are 7 days total with no extra leeway, so this
// is 0 — kept as a named constant (rather than pausing inline on
// trial_ends_at) so a future policy change only needs to touch this line.
const trialGracePeriod = 0 * time.Hour

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
	go c.runEvery(time.Hour, c.pauseDueSites)
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
			if err != nil {
				continue
			}
			contactEmail := ""
			if contact != nil {
				contactEmail = contact.Email
			}
			to := notifyEmail(ctx, c.store, site.OwnerUserID, contactEmail)
			if to == "" {
				continue
			}
			daysLeft := 3
			if kind == "final" {
				daysLeft = 1
			}
			dashboardURL := fmt.Sprintf("%s/dashboard/sites/%s", c.baseURL, site.Slug)
			if err := c.mailer.SendTrialWarning(to, site.BusinessName, dashboardURL, daysLeft); err != nil {
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

// pauseDueSites pauses live sites whose trial ended more than
// trialGracePeriod ago with no paid subscription — nothing else ever
// unpublishes a trial that ran out, so without this every trial site stays
// live free forever.
func (c *Cron) pauseDueSites() {
	ctx := context.Background()
	cutoff := time.Now().UTC().Add(-trialGracePeriod)
	ids, err := postgres.GetSiteIDsDueForTrialPause(ctx, c.store.DB(), cutoff)
	if err != nil {
		slog.Error("trial cron: list sites due for pause", "error", err)
		return
	}
	for _, id := range ids {
		site, err := postgres.GetSiteByID(ctx, c.store.DB(), id)
		if err != nil || site == nil {
			continue
		}
		if err := postgres.SetSiteStatus(ctx, c.store.DB(), id, domain.SiteStatusPaused); err != nil {
			slog.Error("trial cron: pause site", "slug", site.Slug, "error", err)
			continue
		}
		slog.Info("trial site paused", "slug", site.Slug)

		contact, err := postgres.GetSiteContact(ctx, c.store.DB(), id)
		if err != nil {
			continue
		}
		contactEmail := ""
		if contact != nil {
			contactEmail = contact.Email
		}
		to := notifyEmail(ctx, c.store, site.OwnerUserID, contactEmail)
		if to == "" {
			continue
		}
		dashboardURL := fmt.Sprintf("%s/dashboard/sites/%s", c.baseURL, site.Slug)
		if err := c.mailer.SendSitePaused(to, site.BusinessName, dashboardURL); err != nil {
			slog.Error("trial cron: send paused email", "slug", site.Slug, "error", err)
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
	if err != nil {
		return err
	}
	contactEmail := ""
	if contact != nil {
		contactEmail = contact.Email
	}
	to := notifyEmail(ctx, c.store, site.OwnerUserID, contactEmail)
	if to == "" {
		return fmt.Errorf("no notification email on file for site %d", siteID)
	}
	since := time.Now().UTC().Add(-30 * 24 * time.Hour)
	stats, err := c.analytics.GetSiteStats(ctx, siteID, since, site.Timezone)
	if err != nil {
		return fmt.Errorf("get stats: %w", err)
	}
	siteURL := fmt.Sprintf("%s/dashboard/sites/%s", c.baseURL, site.Slug)
	if err := c.mailer.SendAnalyticsDigest(to, site.BusinessName, stats, siteURL); err != nil {
		return fmt.Errorf("send email: %w", err)
	}
	return postgres.UpdateAnalyticsLastSent(ctx, c.store.DB(), siteID)
}
