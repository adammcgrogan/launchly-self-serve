package service

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// Postgres advisory lock keys used to serialize cron passes across server
// instances (see withAdvisoryLock). Trial reminders and trial pauses share a
// key since they're both steps of the same trial-lifecycle sweep; analytics
// digests get a separate key so the two kinds of work don't needlessly
// serialize against each other.
const (
	advisoryLockTrialCron     = 8735100
	advisoryLockAnalyticsCron = 8735101
)

// trialGracePeriod is how long a site stays live after its trial ends
// before being paused. Trials are 7 days total with no extra leeway, so this
// is 0 — kept as a named constant (rather than pausing inline on
// trial_ends_at) so a future policy change only needs to touch this line.
const trialGracePeriod = 0 * time.Hour

// analyticsRetention bounds page_views/site_events — matches the dashboard's
// longest analytics period (see analyticsPeriods' "Max" option in
// internal/web/dashboard.go), so pruning past it never changes what the
// dashboard or digest can show.
const analyticsRetention = 180 * 24 * time.Hour

// stripeEventRetention bounds stripe_events, which only exist to dedupe
// webhook retries — kept well past Stripe's few-day retry window.
const stripeEventRetention = 30 * 24 * time.Hour

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
	go c.runEvery(24*time.Hour, c.pruneOldRecords)
}

func (c *Cron) runEvery(interval time.Duration, fn func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		fn()
	}
}

// withAdvisoryLock runs fn only if this process can claim the given
// Postgres advisory lock, so at most one server instance executes a given
// cron pass at a time — sendDueTrialReminders, pauseDueSites and
// sendDueAnalyticsDigests are each read-then-write across several queries
// (list due sites, send email, mark sent), and without this two instances
// ticking in the same window can both pick up the same due site before
// either marks it sent, double-sending the customer email (#100). If the
// lock is already held (by this process or another), the pass is skipped
// entirely rather than waited on — the next tick will pick it up.
//
// The lock is session-scoped, so it's taken on a single reserved connection
// (*sql.Conn) rather than through the pool: returning a pooled connection to
// the pool doesn't end its Postgres session, so a lock acquired via
// c.store.DB() could easily outlive fn and never get released. fn receives
// that same connection to run its queries on.
func (c *Cron) withAdvisoryLock(ctx context.Context, key int64, fn func(conn *sql.Conn)) {
	conn, err := c.store.DB().Conn(ctx)
	if err != nil {
		slog.Error("cron: acquire db connection", "error", err)
		return
	}
	defer conn.Close()

	var locked bool
	if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil {
		slog.Error("cron: try advisory lock", "key", key, "error", err)
		return
	}
	if !locked {
		return
	}
	defer func() {
		if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, key); err != nil {
			slog.Error("cron: release advisory lock", "key", key, "error", err)
		}
	}()

	fn(conn)
}

func (c *Cron) sendDueTrialReminders() {
	ctx := context.Background()
	c.withAdvisoryLock(ctx, advisoryLockTrialCron, func(conn *sql.Conn) {
		for _, kind := range []string{"first", "final"} {
			ids, err := postgres.GetSiteIDsDueForTrialReminder(ctx, conn, kind)
			if err != nil {
				slog.Error("trial cron: list sites", "kind", kind, "error", err)
				continue
			}
			for _, id := range ids {
				site, err := postgres.GetSiteByID(ctx, conn, id)
				if err != nil || site == nil {
					continue
				}
				billing, err := postgres.GetSiteBilling(ctx, conn, id)
				if err != nil || billing == nil || billing.TrialEndsAt == nil {
					continue
				}
				contact, err := postgres.GetSiteContact(ctx, conn, id)
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
				// Computed from trial_ends_at rather than assumed from kind, so a
				// cron gap that lets a site become due for both "first" and
				// "final" in the same run doesn't send a falsely optimistic
				// days-left count (see #148).
				daysLeft := int(math.Ceil(time.Until(*billing.TrialEndsAt).Hours() / 24))
				if daysLeft < 1 {
					daysLeft = 1
				}
				dashboardURL := fmt.Sprintf("%s/dashboard/sites/%s", c.baseURL, site.Slug)
				if err := c.mailer.SendTrialWarning(to, site.BusinessName, dashboardURL, daysLeft); err != nil {
					slog.Error("trial cron: send reminder", "slug", site.Slug, "kind", kind, "error", err)
					continue
				}
				if err := postgres.MarkTrialReminderSent(ctx, conn, id, kind); err != nil {
					slog.Error("trial cron: mark sent", "slug", site.Slug, "kind", kind, "error", err)
				} else {
					slog.Info("trial reminder sent", "slug", site.Slug, "kind", kind)
				}
			}
		}
	})
}

// pauseDueSites pauses live sites whose trial ended more than
// trialGracePeriod ago with no paid subscription — nothing else ever
// unpublishes a trial that ran out, so without this every trial site stays
// live free forever.
func (c *Cron) pauseDueSites() {
	ctx := context.Background()
	c.withAdvisoryLock(ctx, advisoryLockTrialCron, func(conn *sql.Conn) {
		cutoff := time.Now().UTC().Add(-trialGracePeriod)
		ids, err := postgres.GetSiteIDsDueForTrialPause(ctx, conn, cutoff)
		if err != nil {
			slog.Error("trial cron: list sites due for pause", "error", err)
			return
		}
		for _, id := range ids {
			site, err := postgres.GetSiteByID(ctx, conn, id)
			if err != nil || site == nil {
				continue
			}
			if err := postgres.SetSiteStatus(ctx, conn, id, domain.SiteStatusPaused); err != nil {
				slog.Error("trial cron: pause site", "slug", site.Slug, "error", err)
				continue
			}
			slog.Info("trial site paused", "slug", site.Slug)

			contact, err := postgres.GetSiteContact(ctx, conn, id)
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
	})
}

// pruneOldRecords deletes rows past their retention window from the
// high-volume tables that are never pruned otherwise — nothing else bounds
// their growth.
func (c *Cron) pruneOldRecords() {
	ctx := context.Background()
	if err := postgres.PruneOldPageViews(ctx, c.store.DB(), time.Now().UTC().Add(-analyticsRetention)); err != nil {
		slog.Error("prune cron: page_views", "error", err)
	}
	if err := postgres.PruneOldSiteEvents(ctx, c.store.DB(), time.Now().UTC().Add(-analyticsRetention)); err != nil {
		slog.Error("prune cron: site_events", "error", err)
	}
	if err := postgres.PruneOldStripeEvents(ctx, c.store.DB(), time.Now().UTC().Add(-stripeEventRetention)); err != nil {
		slog.Error("prune cron: stripe_events", "error", err)
	}
}

func (c *Cron) sendDueAnalyticsDigests() {
	ctx := context.Background()
	c.withAdvisoryLock(ctx, advisoryLockAnalyticsCron, func(conn *sql.Conn) {
		ids, err := postgres.GetSiteIDsDueForAnalytics(ctx, conn)
		if err != nil {
			slog.Error("analytics cron: list sites", "error", err)
			return
		}
		for _, id := range ids {
			if err := c.SendAnalyticsReport(ctx, id); err != nil {
				slog.Error("analytics cron: send report", "site_id", id, "error", err)
			}
		}
	})
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
