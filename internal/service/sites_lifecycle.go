package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/lib/pq"
)

// Errors returned by RenameSlug — web handlers show these directly to the
// site owner, so their text is user-facing.
var (
	ErrSlugInvalid     = errors.New("enter a valid address.")
	ErrSlugReserved    = errors.New("that address is reserved.")
	ErrSlugTaken       = errors.New("that address is already taken.")
	ErrSlugRateLimited = errors.New("you can only change your address once per day.")
)

// ErrSitePaused is returned by Publish when the site was paused by the
// trial cron — publishing isn't a free way back online, it has to go
// through checkout so the reactivation actually resolves the unpaid trial.
var ErrSitePaused = errors.New("your site is paused — upgrade to reactivate it.")

// ErrSiteLimitReached is returned by CreateSite when an account with no
// Pro-plan site tries to create more than one site. Plan is tracked per
// site, not per account, so the cap is: Starter/trial accounts get 1 site;
// having Pro on any existing site lifts the cap to unlimited.
var ErrSiteLimitReached = errors.New("your plan is limited to 1 site — upgrade an existing site to Pro to add more.")

var (
	slugStripRe = regexp.MustCompile(`['\x60]`)
	slugCharsRe = regexp.MustCompile(`[^a-z0-9]+`)
)

// reservedSlugs can't be claimed as a site's address — they're platform
// routes or would be confusing as a subdomain.
var reservedSlugs = map[string]bool{
	"www": true, "api": true, "dashboard": true, "superadmin": true, "static": true,
}

// slugRenameCooldown limits how often an owner can rename their site's
// slug, to discourage squatting/churn on desirable addresses.
const slugRenameCooldown = 24 * time.Hour

func toSlug(s string) string {
	s = strings.ToLower(s)
	s = slugStripRe.ReplaceAllString(s, "")
	s = slugCharsRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// ToSlug exposes the slug normalization used when generating site slugs, so
// callers can check whether an arbitrary string (e.g. a request host) is
// already a well-formed slug.
func ToSlug(s string) string {
	return toSlug(s)
}

// CreateSiteInput is the fully-filled-in builder wizard form.
type CreateSiteInput struct {
	OwnerUserID  uuid.UUID
	BusinessName string
	Tagline      string
	About        string
	LogoURL      string
	CTAText      string
	TemplateID   string
	Palette      string
	HeadingFont  string
	Timezone     string

	Contact        domain.SiteContact
	SocialLinks    []domain.SocialLink
	Services       []domain.Service
	Certifications []domain.Certification
	Testimonials   []domain.Testimonial
	GalleryImages  []domain.GalleryImage
	FAQItems       []domain.FAQItem
	StaffMembers   []domain.StaffMember
	BusinessHours  []domain.BusinessHours
}

// maxCreateSiteSlugAttempts bounds how many times CreateSite retries after
// losing a slug-uniqueness race, so a pathological case fails loudly instead
// of looping forever.
const maxCreateSiteSlugAttempts = 5

// CreateSite generates a unique slug, inserts the site and all related rows
// in one transaction, and sets it live immediately with a 7-day Starter
// trial — there is no draft/review step. uniqueSlug's read happens outside the
// insert transaction, so two concurrent creates for the same business name
// can both pick the same slug; if the insert then loses that race on the
// slug's unique constraint, we regenerate and retry rather than surfacing a
// 500. The per-account site cap, by contrast, is enforced inside
// createSiteTx itself (behind an advisory lock), so it can't be raced the
// same way — see canCreateSite.
func (s *Sites) CreateSite(ctx context.Context, in CreateSiteInput) (*domain.SiteAggregate, error) {
	if err := validateSiteContent(in.BusinessName, in.Tagline, in.About, in.LogoURL, in.CTAText, in.Contact, in.SocialLinks, in.Services, in.Certifications, in.Testimonials, in.GalleryImages, in.FAQItems, in.StaffMembers, nil); err != nil {
		return nil, err
	}
	if err := validateBusinessHours(in.BusinessHours); err != nil {
		return nil, err
	}

	var siteID int
	for attempt := 1; ; attempt++ {
		slug, err := s.uniqueSlug(ctx, in.BusinessName)
		if err != nil {
			return nil, fmt.Errorf("generate slug: %w", err)
		}

		siteID, err = s.createSiteTx(ctx, in, slug)
		if err == nil {
			slog.Info("site created", "site_id", siteID, "owner_id", in.OwnerUserID, "slug", slug)
			break
		}
		if errors.Is(err, ErrSiteLimitReached) {
			return nil, err
		}
		if !isUniqueSlugViolation(err) || attempt >= maxCreateSiteSlugAttempts {
			return nil, err
		}
	}

	return s.GetSiteAggregate(ctx, siteID)
}

// isUniqueSlugViolation reports whether err is a Postgres unique-constraint
// violation (23505) on the sites table's slug column.
func isUniqueSlugViolation(err error) bool {
	var pqErr *pq.Error
	if !errors.As(err, &pqErr) {
		return false
	}
	return pqErr.Code == "23505" && pqErr.Constraint == "sites_slug_key"
}

// createSiteTx inserts a site and all its related rows in one transaction.
func (s *Sites) createSiteTx(ctx context.Context, in CreateSiteInput, slug string) (int, error) {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Take an advisory lock scoped to this owner before checking the cap, so
	// a second concurrent CreateSite for the same account blocks here until
	// the first transaction commits or rolls back, instead of both reading
	// the pre-insert site count and both passing the check (#214).
	if err := postgres.LockOwnerForSiteCreate(ctx, tx, in.OwnerUserID); err != nil {
		return 0, fmt.Errorf("lock owner: %w", err)
	}
	allowed, err := s.canCreateSite(ctx, tx, in.OwnerUserID)
	if err != nil {
		return 0, fmt.Errorf("check site limit: %w", err)
	}
	if !allowed {
		return 0, ErrSiteLimitReached
	}

	site := &domain.Site{
		OwnerUserID:  in.OwnerUserID,
		Slug:         slug,
		BusinessName: in.BusinessName,
		Tagline:      in.Tagline,
		About:        in.About,
		LogoURL:      in.LogoURL,
		CTAText:      in.CTAText,
		TemplateID:   in.TemplateID,
		Palette:      in.Palette,
		HeadingFont:  in.HeadingFont,
		Timezone:     in.Timezone,
	}
	siteID, err := postgres.CreateSite(ctx, tx, site)
	if err != nil {
		return 0, fmt.Errorf("create site: %w", err)
	}

	if err := postgres.CreateSiteBilling(ctx, tx, siteID, domain.PlanStarter); err != nil {
		return 0, fmt.Errorf("create billing: %w", err)
	}
	in.Contact.SiteID = siteID
	if err := postgres.UpsertSiteContact(ctx, tx, &in.Contact); err != nil {
		return 0, fmt.Errorf("save contact: %w", err)
	}
	if err := postgres.UpsertSiteAnalyticsSettings(ctx, tx, &domain.SiteAnalyticsSettings{SiteID: siteID, AnalyticsFrequency: "off"}); err != nil {
		return 0, fmt.Errorf("save analytics settings: %w", err)
	}
	if err := postgres.ReplaceSiteSocialLinks(ctx, tx, siteID, in.SocialLinks); err != nil {
		return 0, fmt.Errorf("save social links: %w", err)
	}
	if err := postgres.ReplaceSiteServices(ctx, tx, siteID, in.Services); err != nil {
		return 0, fmt.Errorf("save services: %w", err)
	}
	if err := postgres.ReplaceSiteCertifications(ctx, tx, siteID, in.Certifications); err != nil {
		return 0, fmt.Errorf("save certifications: %w", err)
	}
	if err := postgres.ReplaceSiteTestimonials(ctx, tx, siteID, in.Testimonials); err != nil {
		return 0, fmt.Errorf("save testimonials: %w", err)
	}
	if err := postgres.ReplaceSiteGalleryImages(ctx, tx, siteID, in.GalleryImages); err != nil {
		return 0, fmt.Errorf("save gallery: %w", err)
	}
	if err := postgres.ReplaceSiteFAQItems(ctx, tx, siteID, in.FAQItems); err != nil {
		return 0, fmt.Errorf("save FAQ items: %w", err)
	}
	if err := postgres.ReplaceSiteStaffMembers(ctx, tx, siteID, in.StaffMembers); err != nil {
		return 0, fmt.Errorf("save staff members: %w", err)
	}
	if err := postgres.ReplaceSiteBusinessHours(ctx, tx, siteID, in.BusinessHours); err != nil {
		return 0, fmt.Errorf("save business hours: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return siteID, nil
}

// canCreateSite enforces the per-account site cap: an account with no
// Pro-plan site is limited to 1 site total; having Pro on any existing site
// lifts the cap, since plan is tracked per site rather than per account. tx
// must be the same transaction createSiteTx will insert on, and the caller
// must have already taken LockOwnerForSiteCreate on it — otherwise this
// count-then-check is racy across concurrent CreateSite calls (#214).
func (s *Sites) canCreateSite(ctx context.Context, tx *sql.Tx, ownerID uuid.UUID) (bool, error) {
	count, err := postgres.CountSitesByOwner(ctx, tx, ownerID)
	if err != nil {
		return false, err
	}
	if count == 0 {
		return true, nil
	}
	return postgres.OwnerHasProSite(ctx, tx, ownerID)
}

func (s *Sites) uniqueSlug(ctx context.Context, businessName string) (string, error) {
	base := toSlug(businessName)
	if base == "" {
		base = "site"
	}
	slug := base
	for i := 2; ; i++ {
		if reservedSlugs[slug] {
			slug = fmt.Sprintf("%s-%d", base, i)
			continue
		}
		existing, err := postgres.GetSiteBySlug(ctx, s.store.DB(), slug)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
}

// RenameSlug changes a site's subdomain, recording the old slug in
// slug_redirects (next to sites.slug, in one transaction) so links to it
// keep working via a 301 in serveSiteBySlug. Limited to once per day per
// site to stop slug squatting/churn.
func (s *Sites) RenameSlug(ctx context.Context, siteID int, newSlugRaw string) (string, error) {
	newSlug := toSlug(newSlugRaw)
	if newSlug == "" {
		return "", ErrSlugInvalid
	}
	if reservedSlugs[newSlug] {
		return "", ErrSlugReserved
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	current, err := postgres.GetSiteByID(ctx, tx, siteID)
	if err != nil {
		return "", fmt.Errorf("load site: %w", err)
	}
	if current == nil {
		return "", fmt.Errorf("site %d not found", siteID)
	}
	if current.Slug == newSlug {
		return newSlug, nil
	}
	if current.SlugChangedAt != nil && time.Since(*current.SlugChangedAt) < slugRenameCooldown {
		return "", ErrSlugRateLimited
	}

	taken, err := postgres.SlugInUse(ctx, tx, newSlug)
	if err != nil {
		return "", fmt.Errorf("check slug: %w", err)
	}
	if taken {
		return "", ErrSlugTaken
	}

	if err := postgres.CreateSlugRedirect(ctx, tx, current.Slug, siteID); err != nil {
		return "", fmt.Errorf("save redirect: %w", err)
	}
	if err := postgres.RenameSiteSlug(ctx, tx, siteID, newSlug); err != nil {
		return "", fmt.Errorf("rename slug: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	s.invalidateAggregate(siteID)
	return newSlug, nil
}

// ResolveSlugRedirect looks up the current slug an old, renamed-away-from
// slug now points to. Used by the public site handler to 301 stale links.
func (s *Sites) ResolveSlugRedirect(ctx context.Context, oldSlug string) (string, bool, error) {
	siteID, ok, err := postgres.GetSlugRedirectSiteID(ctx, s.store.DB(), oldSlug)
	if err != nil || !ok {
		return "", false, err
	}
	site, err := postgres.GetSiteByID(ctx, s.store.DB(), siteID)
	if err != nil || site == nil {
		return "", false, err
	}
	return site.Slug, true, nil
}

// Actor values identify who triggered an audited action (Unpublish,
// Delete) — logged alongside it so it's possible to tell from logs whether
// an action was owner self-service or superadmin intervention.
const (
	ActorOwner      = "owner"
	ActorSuperadmin = "superadmin"
)

// Publish and Unpublish let an owner take their own site up/down at will —
// there is no admin approval gate on either direction. A paused site is the
// exception: it can only come back via checkout (see ErrSitePaused).
func (s *Sites) Publish(ctx context.Context, siteID int) error {
	site, err := postgres.GetSiteByID(ctx, s.store.DB(), siteID)
	if err != nil {
		return err
	}
	if site == nil {
		return fmt.Errorf("site %d not found", siteID)
	}
	if site.Status == domain.SiteStatusPaused {
		return ErrSitePaused
	}
	err = postgres.SetSiteStatus(ctx, s.store.DB(), siteID, domain.SiteStatusLive)
	if err == nil {
		s.invalidateAggregate(siteID)
	}
	return err
}

// Unpublish takes actor identifying who triggered it (ActorOwner or
// ActorSuperadmin) so the log line can distinguish an owner's self-service
// unpublish from a superadmin's abuse-handling intervention.
func (s *Sites) Unpublish(ctx context.Context, siteID int, actor string) error {
	site, err := postgres.GetSiteByID(ctx, s.store.DB(), siteID)
	if err != nil {
		return err
	}
	if site == nil {
		return fmt.Errorf("site %d not found", siteID)
	}
	if site.Status == domain.SiteStatusPaused {
		return ErrSitePaused
	}
	err = postgres.SetSiteStatus(ctx, s.store.DB(), siteID, domain.SiteStatusDraft)
	if err == nil {
		s.invalidateAggregate(siteID)
		slog.Info("site unpublished", "site_id", siteID, "actor", actor)
	}
	return err
}

// Delete removes a site and, if it had an active paid subscription,
// cancels it in Stripe first — otherwise the customer keeps being billed
// for a site that no longer exists, with no dashboard page left to cancel
// it from themselves. actor identifies who triggered it (ActorOwner or
// ActorSuperadmin) so the log line can distinguish an owner's self-service
// deletion from a superadmin's abuse-handling intervention.
func (s *Sites) Delete(ctx context.Context, siteID int, actor string) error {
	if err := s.billing.CancelSubscriptionIfActive(ctx, siteID); err != nil {
		return fmt.Errorf("cancel subscription: %w", err)
	}
	site, err := postgres.GetSiteByID(ctx, s.store.DB(), siteID)
	if err != nil {
		return fmt.Errorf("load site: %w", err)
	}
	if site != nil && site.CustomDomainCFID != "" {
		if err := s.cf.DeleteCustomHostname(ctx, site.CustomDomainCFID); err != nil {
			return fmt.Errorf("remove custom domain from cloudflare: %w", err)
		}
	}
	if err := postgres.DeleteSite(ctx, s.store.DB(), siteID); err != nil {
		return err
	}
	s.invalidateAggregate(siteID)
	slog.Info("site deleted", "site_id", siteID, "actor", actor)
	return nil
}
