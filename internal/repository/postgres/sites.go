package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

const siteColumns = `id, owner_user_id, slug, business_name, tagline, about, logo_url, cta_text,
	template_id, form_type, palette, heading_font, brand_color, status, created_at, published_at, updated_at, slug_changed_at,
	custom_domain, custom_domain_status, custom_domain_cf_id, custom_domain_added_at, timezone,
	meta_title, meta_description, og_image_url, video_url, is_demo, thank_you_message, redirect_url,
	document_title, document_url`

func scanSite(row scanner) (*domain.Site, error) {
	var s domain.Site
	var customDomain, customDomainCFID sql.NullString
	err := row.Scan(
		&s.ID, &s.OwnerUserID, &s.Slug, &s.BusinessName, &s.Tagline, &s.About, &s.LogoURL, &s.CTAText,
		&s.TemplateID, &s.FormType, &s.Palette, &s.HeadingFont, &s.BrandColor, &s.Status, &s.CreatedAt, &s.PublishedAt, &s.UpdatedAt, &s.SlugChangedAt,
		&customDomain, &s.CustomDomainStatus, &customDomainCFID, &s.CustomDomainAddedAt, &s.Timezone,
		&s.MetaTitle, &s.MetaDescription, &s.OgImageURL, &s.VideoURL, &s.IsDemo, &s.ThankYouMessage, &s.RedirectURL,
		&s.DocumentTitle, &s.DocumentURL,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.CustomDomain = customDomain.String
	s.CustomDomainCFID = customDomainCFID.String
	return &s, nil
}

// scanSiteRowsWithCount is scanSite plus a trailing COUNT(*) OVER()
// column, for paginated queries that report a total alongside the page.
func scanSiteRowsWithCount(rows *sql.Rows) (*domain.Site, int, error) {
	var s domain.Site
	var customDomain, customDomainCFID sql.NullString
	var total int
	err := rows.Scan(
		&s.ID, &s.OwnerUserID, &s.Slug, &s.BusinessName, &s.Tagline, &s.About, &s.LogoURL, &s.CTAText,
		&s.TemplateID, &s.FormType, &s.Palette, &s.HeadingFont, &s.BrandColor, &s.Status, &s.CreatedAt, &s.PublishedAt, &s.UpdatedAt, &s.SlugChangedAt,
		&customDomain, &s.CustomDomainStatus, &customDomainCFID, &s.CustomDomainAddedAt, &s.Timezone,
		&s.MetaTitle, &s.MetaDescription, &s.OgImageURL, &s.VideoURL, &s.IsDemo, &s.ThankYouMessage, &s.RedirectURL,
		&s.DocumentTitle, &s.DocumentURL,
		&total,
	)
	s.CustomDomain = customDomain.String
	s.CustomDomainCFID = customDomainCFID.String
	return &s, total, err
}

// CreateSite inserts a site's core row. Status is set to live and
// published_at to now — sites go live immediately, there is no draft/review
// step in the self-serve flow.
func CreateSite(ctx context.Context, q querier, site *domain.Site) (int, error) {
	now := time.Now().UTC()
	err := q.QueryRowContext(ctx, `
		INSERT INTO sites (owner_user_id, slug, business_name, tagline, about, logo_url, cta_text,
		                   template_id, palette, heading_font, status, published_at, timezone, is_demo)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'live', $11, $12, $13)
		RETURNING id
	`, site.OwnerUserID, site.Slug, site.BusinessName, site.Tagline, site.About, site.LogoURL, site.CTAText,
		site.TemplateID, site.Palette, site.HeadingFont, now, site.Timezone, site.IsDemo,
	).Scan(&site.ID)
	return site.ID, err
}

// ListDemoSites returns the seeded showcase demo sites (one per template),
// for the public /templates gallery's live-example links.
func ListDemoSites(ctx context.Context, q querier) ([]domain.Site, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE is_demo ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sites []domain.Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, *s)
	}
	return sites, rows.Err()
}

func GetSiteByID(ctx context.Context, q querier, id int) (*domain.Site, error) {
	return scanSite(q.QueryRowContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE id = $1`, id))
}

func GetSiteBySlug(ctx context.Context, q querier, slug string) (*domain.Site, error) {
	return scanSite(q.QueryRowContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE slug = $1`, slug))
}

// CountSitesByOwner returns how many sites an account owns, used to enforce
// the per-account site cap (see service.Sites.canCreateSite).
func CountSitesByOwner(ctx context.Context, q querier, ownerID uuid.UUID) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM sites WHERE owner_user_id = $1`, ownerID).Scan(&n)
	return n, err
}

// LockOwnerForSiteCreate takes a Postgres transaction-scoped advisory lock
// keyed on ownerID, serializing concurrent CreateSite calls for the same
// account. It must be called on the *sql.Tx that will also perform the cap
// check and insert (the lock auto-releases on commit/rollback) — this closes
// the race where two concurrent creates both read the pre-insert site count
// as 0 and both pass service.Sites.canCreateSite (see #214). hashtext folds
// the uuid down to the int4 pg_advisory_xact_lock expects; a same-owner hash
// collision only ever over-serializes (blocks an unrelated create briefly),
// never under-serializes, so it's safe to use as the lock key.
func LockOwnerForSiteCreate(ctx context.Context, q querier, ownerID uuid.UUID) error {
	_, err := q.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, ownerID.String())
	return err
}

// GetPrimarySiteByOwner returns an account's oldest site — the representative
// one for account-level messaging (billing emails, trial reminders), where
// there is one account but possibly several sites (#338). Returns nil, nil
// if the account has no sites.
func GetPrimarySiteByOwner(ctx context.Context, q querier, ownerID uuid.UUID) (*domain.Site, error) {
	site, err := scanSite(q.QueryRowContext(ctx,
		`SELECT `+siteColumns+` FROM sites WHERE owner_user_id = $1 ORDER BY created_at LIMIT 1`, ownerID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return site, err
}

// ListSiteIDsByOwner returns every site ID an account owns, for the
// account-wide cache invalidation that follows a billing change.
func ListSiteIDsByOwner(ctx context.Context, q querier, ownerID uuid.UUID) ([]int, error) {
	rows, err := q.QueryContext(ctx, `SELECT id FROM sites WHERE owner_user_id = $1`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func ListSitesByOwner(ctx context.Context, q querier, ownerID uuid.UUID) ([]domain.Site, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE owner_user_id = $1 ORDER BY created_at DESC`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sites []domain.Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, *s)
	}
	return sites, rows.Err()
}

// SiteFilter pages through ListAllSitesFiltered's results with Limit/Offset.
type SiteFilter struct {
	Limit  int
	Offset int
}

// scanSiteWithBillingRowsWithCount is scanSiteRowsWithCount plus the owning
// account's billing snapshot (nullable via the LEFT JOIN — an account should
// always have a billing row once it has a site, but this doesn't assume it).
func scanSiteWithBillingRowsWithCount(rows *sql.Rows) (*domain.SiteWithBilling, int, error) {
	var sb domain.SiteWithBilling
	var customDomain, customDomainCFID sql.NullString
	var plan, paymentStatus, stripeCustomerID, stripeSessionID, stripeSubscriptionID sql.NullString
	var paidAt, trialEndsAt, trialReminderSentAt, trialFinalReminderSentAt sql.NullTime
	var paymentFailedAt, dunningReminder1SentAt, dunningReminder2SentAt, dunningFinalWarningSentAt sql.NullTime
	var total int
	err := rows.Scan(
		&sb.ID, &sb.OwnerUserID, &sb.Slug, &sb.BusinessName, &sb.Tagline, &sb.About, &sb.LogoURL, &sb.CTAText,
		&sb.TemplateID, &sb.FormType, &sb.Palette, &sb.HeadingFont, &sb.BrandColor, &sb.Status, &sb.CreatedAt, &sb.PublishedAt, &sb.UpdatedAt, &sb.SlugChangedAt,
		&customDomain, &sb.CustomDomainStatus, &customDomainCFID, &sb.CustomDomainAddedAt, &sb.Timezone,
		&sb.MetaTitle, &sb.MetaDescription, &sb.OgImageURL, &sb.VideoURL, &sb.IsDemo, &sb.ThankYouMessage, &sb.RedirectURL,
		&sb.DocumentTitle, &sb.DocumentURL,
		&plan, &paymentStatus, &stripeCustomerID, &stripeSessionID, &stripeSubscriptionID,
		&paidAt, &trialEndsAt, &trialReminderSentAt, &trialFinalReminderSentAt,
		&paymentFailedAt, &dunningReminder1SentAt, &dunningReminder2SentAt, &dunningFinalWarningSentAt,
		&total,
	)
	if err != nil {
		return nil, 0, err
	}
	sb.CustomDomain = customDomain.String
	sb.CustomDomainCFID = customDomainCFID.String
	sb.Billing = domain.AccountBilling{
		OwnerUserID:          sb.OwnerUserID,
		Plan:                 domain.Plan(plan.String),
		PaymentStatus:        domain.PaymentStatus(paymentStatus.String),
		StripeCustomerID:     stripeCustomerID.String,
		StripeSessionID:      stripeSessionID.String,
		StripeSubscriptionID: stripeSubscriptionID.String,
	}
	if paidAt.Valid {
		sb.Billing.PaidAt = &paidAt.Time
	}
	if trialEndsAt.Valid {
		sb.Billing.TrialEndsAt = &trialEndsAt.Time
	}
	if trialReminderSentAt.Valid {
		sb.Billing.TrialReminderSentAt = &trialReminderSentAt.Time
	}
	if trialFinalReminderSentAt.Valid {
		sb.Billing.TrialFinalReminderSentAt = &trialFinalReminderSentAt.Time
	}
	if paymentFailedAt.Valid {
		sb.Billing.PaymentFailedAt = &paymentFailedAt.Time
	}
	if dunningReminder1SentAt.Valid {
		sb.Billing.DunningReminder1SentAt = &dunningReminder1SentAt.Time
	}
	if dunningReminder2SentAt.Valid {
		sb.Billing.DunningReminder2SentAt = &dunningReminder2SentAt.Time
	}
	if dunningFinalWarningSentAt.Valid {
		sb.Billing.DunningFinalWarningSentAt = &dunningFinalWarningSentAt.Time
	}
	return &sb, total, nil
}

// ListAllSitesFiltered lists a page of sites, newest first, along with the
// total count of sites (for pagination) and each site's owning account's
// billing snapshot (trial/payment status), computed in the same query via
// COUNT(*) OVER() and a LEFT JOIN. Sites sharing an owner therefore share a
// billing snapshot (#338). Used by the superadmin dashboard.
func ListAllSitesFiltered(ctx context.Context, q querier, filter SiteFilter) ([]domain.SiteWithBilling, int, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}

	rows, err := q.QueryContext(ctx, `
		SELECT `+siteColumns+`, `+accountBillingColumns+`, COUNT(*) OVER() AS total_count
		FROM sites
		LEFT JOIN account_billing b ON b.owner_user_id = sites.owner_user_id
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sites []domain.SiteWithBilling
	total := 0
	for rows.Next() {
		s, t, err := scanSiteWithBillingRowsWithCount(rows)
		if err != nil {
			return nil, 0, err
		}
		total = t
		sites = append(sites, *s)
	}
	return sites, total, rows.Err()
}

// GetPlatformStats returns platform-wide site/plan counts, for the
// superadmin dashboard's stats view. Plans are per-account (#338), so the
// plan counters count distinct accounts — a Pro account with four sites is
// one Pro subscription, not four.
func GetPlatformStats(ctx context.Context, q querier) (domain.PlatformStats, error) {
	var s domain.PlatformStats
	err := q.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE s.status = 'live'),
			COUNT(*) FILTER (WHERE s.status = 'draft'),
			COUNT(*) FILTER (WHERE s.status = 'paused'),
			COUNT(DISTINCT s.owner_user_id) FILTER (WHERE b.plan = 'starter'),
			COUNT(DISTINCT s.owner_user_id) FILTER (WHERE b.plan = 'pro' AND b.payment_status = 'paid'),
			COUNT(DISTINCT s.owner_user_id) FILTER (WHERE b.payment_status = 'trialing'),
			COUNT(*) FILTER (WHERE s.created_at >= now() - interval '7 days'),
			COUNT(*) FILTER (WHERE s.created_at >= now() - interval '30 days')
		FROM sites s
		LEFT JOIN account_billing b ON b.owner_user_id = s.owner_user_id
		WHERE NOT s.is_demo
	`).Scan(
		&s.TotalSites, &s.LiveSites, &s.DraftSites, &s.PausedSites,
		&s.StarterPlan, &s.ProPlan, &s.TrialingSites,
		&s.SignupsThisWeek, &s.SignupsThisMonth,
	)
	return s, err
}

// ListLiveSites returns every published site, for the public sitemap.
func ListLiveSites(ctx context.Context, q querier) ([]domain.Site, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE status = 'live' ORDER BY published_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sites []domain.Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, *s)
	}
	return sites, rows.Err()
}

// UpdateSiteContent saves the editable core fields (not appearance/template/status).
func UpdateSiteContent(ctx context.Context, q querier, site *domain.Site) error {
	_, err := q.ExecContext(ctx, `
		UPDATE sites SET business_name = $1, tagline = $2, about = $3, logo_url = $4, cta_text = $5, timezone = $6,
			meta_title = $7, meta_description = $8, og_image_url = $9, video_url = $10,
			thank_you_message = $11, redirect_url = $12, document_title = $13, document_url = $14, updated_at = now()
		WHERE id = $15
	`, site.BusinessName, site.Tagline, site.About, site.LogoURL, site.CTAText, site.Timezone,
		site.MetaTitle, site.MetaDescription, site.OgImageURL, site.VideoURL,
		site.ThankYouMessage, site.RedirectURL, site.DocumentTitle, site.DocumentURL, site.ID)
	return err
}

func UpdateSiteAppearance(ctx context.Context, q querier, id int, palette, headingFont, brandColor string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET palette = $1, heading_font = $2, brand_color = $3, updated_at = now() WHERE id = $4`, palette, headingFont, brandColor, id)
	return err
}

func UpdateSiteTemplate(ctx context.Context, q querier, id int, templateID string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET template_id = $1, updated_at = now() WHERE id = $2`, templateID, id)
	return err
}

func UpdateSiteFormType(ctx context.Context, q querier, id int, formType domain.FormType) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET form_type = $1, updated_at = now() WHERE id = $2`, formType, id)
	return err
}

// RenameSiteSlug updates a site's live slug and stamps slug_changed_at, used
// to enforce the once-per-day rename limit.
func RenameSiteSlug(ctx context.Context, q querier, id int, slug string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET slug = $1, slug_changed_at = now(), updated_at = now() WHERE id = $2`, slug, id)
	return err
}

// SlugInUse reports whether slug is already a live site's slug or a
// redirect's old slug, so renames can't collide with either.
func SlugInUse(ctx context.Context, q querier, slug string) (bool, error) {
	var inUse bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM sites WHERE slug = $1)
		OR EXISTS(SELECT 1 FROM slug_redirects WHERE old_slug = $1)
	`, slug).Scan(&inUse)
	return inUse, err
}

func SetSiteStatus(ctx context.Context, q querier, id int, status domain.SiteStatus) error {
	switch status {
	case domain.SiteStatusLive:
		_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'live', published_at = now(), updated_at = now() WHERE id = $1`, id)
		return err
	case domain.SiteStatusPaused:
		_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'paused', updated_at = now() WHERE id = $1`, id)
		return err
	default:
		_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'draft', updated_at = now() WHERE id = $1`, id)
		return err
	}
}

// SetSitesPaused pauses every given site ID in one round trip, for the
// trial-pause cron sweep — see GetSitesDueForTrialPause (#218).
// SetSitesPausedByOwner pauses every live site an account owns. Plans are
// per-account (#338), so a trial expiring or a subscription ending takes the
// whole account offline together rather than one site at a time.
func SetSitesPausedByOwner(ctx context.Context, q querier, ownerID uuid.UUID) error {
	_, err := q.ExecContext(ctx,
		`UPDATE sites SET status = 'paused', updated_at = now()
		 WHERE owner_user_id = $1 AND status = 'live'`, ownerID)
	return err
}

// ReactivateSitesByOwner brings every site an account paused for non-payment
// back online when they pay. It deliberately only touches 'paused' sites, so
// a site the owner themselves unpublished stays a draft.
func ReactivateSitesByOwner(ctx context.Context, q querier, ownerID uuid.UUID) error {
	_, err := q.ExecContext(ctx,
		`UPDATE sites SET status = 'live', updated_at = now()
		 WHERE owner_user_id = $1 AND status = 'paused'`, ownerID)
	return err
}

func SetSitesPaused(ctx context.Context, q querier, ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'paused', updated_at = now() WHERE id = ANY($1)`, pq.Array(ids))
	return err
}

func DeleteSite(ctx context.Context, q querier, id int) error {
	_, err := q.ExecContext(ctx, `DELETE FROM sites WHERE id = $1`, id)
	return err
}

// GetSiteByCustomDomain looks up the site that owns an active custom
// domain, for routing requests whose Host isn't a *.DOMAIN subdomain.
func GetSiteByCustomDomain(ctx context.Context, q querier, host string) (*domain.Site, error) {
	return scanSite(q.QueryRowContext(ctx,
		`SELECT `+siteColumns+` FROM sites WHERE custom_domain = $1 AND custom_domain_status = 'active'`, host))
}

// CustomDomainInUse reports whether customDomain is already attached to any site.
func CustomDomainInUse(ctx context.Context, q querier, customDomain string) (bool, error) {
	var inUse bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sites WHERE custom_domain = $1)`, customDomain).Scan(&inUse)
	return inUse, err
}

// SetCustomDomain attaches a pending custom domain to a site.
func SetCustomDomain(ctx context.Context, q querier, id int, customDomain, cfID string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE sites
		SET custom_domain = $1, custom_domain_status = 'pending', custom_domain_cf_id = $2,
		    custom_domain_added_at = now(), updated_at = now()
		WHERE id = $3
	`, customDomain, cfID, id)
	return err
}

// UpdateCustomDomainStatus updates a site's custom domain verification status.
func UpdateCustomDomainStatus(ctx context.Context, q querier, id int, status domain.CustomDomainStatus) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET custom_domain_status = $1, updated_at = now() WHERE id = $2`, status, id)
	return err
}

// ClearCustomDomain removes a site's custom domain entirely.
func ClearCustomDomain(ctx context.Context, q querier, id int) error {
	_, err := q.ExecContext(ctx, `
		UPDATE sites
		SET custom_domain = NULL, custom_domain_status = 'none', custom_domain_cf_id = NULL,
		    custom_domain_added_at = NULL, updated_at = now()
		WHERE id = $1
	`, id)
	return err
}
