package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

// Sites owns the create/edit/publish lifecycle of a site and assembling the
// full SiteAggregate from the many tables it spans.
type Sites struct {
	store   *postgres.Store
	billing *Billing
}

func NewSites(store *postgres.Store, billing *Billing) *Sites {
	return &Sites{store: store, billing: billing}
}

var (
	slugStripRe = regexp.MustCompile(`['\x60]`)
	slugCharsRe = regexp.MustCompile(`[^a-z0-9]+`)
	e164Re      = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)
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

// Errors returned by UpdateNotifySettings — web handlers show these
// directly to the site owner, so their text is user-facing.
var (
	ErrNotifyNotPro        = errors.New("SMS lead alerts are a Pro feature.")
	ErrNotifyInvalidNumber = errors.New("enter your mobile number in international format, e.g. +447700900123.")
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

	Contact        domain.SiteContact
	SocialLinks    []domain.SocialLink
	Services       []domain.Service
	Certifications []domain.Certification
	Testimonials   []domain.Testimonial
	GalleryImages  []domain.GalleryImage
	BusinessHours  []domain.BusinessHours
}

// CreateSite generates a unique slug, inserts the site and all related rows
// in one transaction, and sets it live immediately with a 14-day trial —
// there is no draft/review step.
func (s *Sites) CreateSite(ctx context.Context, in CreateSiteInput) (*domain.SiteAggregate, error) {
	slug, err := s.uniqueSlug(ctx, in.BusinessName)
	if err != nil {
		return nil, fmt.Errorf("generate slug: %w", err)
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

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
	}
	siteID, err := postgres.CreateSite(ctx, tx, site)
	if err != nil {
		return nil, fmt.Errorf("create site: %w", err)
	}

	if err := postgres.CreateSiteBilling(ctx, tx, siteID, domain.PlanStarter); err != nil {
		return nil, fmt.Errorf("create billing: %w", err)
	}
	in.Contact.SiteID = siteID
	if err := postgres.UpsertSiteContact(ctx, tx, &in.Contact); err != nil {
		return nil, fmt.Errorf("save contact: %w", err)
	}
	if err := postgres.UpsertSiteAnalyticsSettings(ctx, tx, &domain.SiteAnalyticsSettings{SiteID: siteID, AnalyticsFrequency: "off"}); err != nil {
		return nil, fmt.Errorf("save analytics settings: %w", err)
	}
	if err := postgres.ReplaceSiteSocialLinks(ctx, tx, siteID, in.SocialLinks); err != nil {
		return nil, fmt.Errorf("save social links: %w", err)
	}
	if err := postgres.ReplaceSiteServices(ctx, tx, siteID, in.Services); err != nil {
		return nil, fmt.Errorf("save services: %w", err)
	}
	if err := postgres.ReplaceSiteCertifications(ctx, tx, siteID, in.Certifications); err != nil {
		return nil, fmt.Errorf("save certifications: %w", err)
	}
	if err := postgres.ReplaceSiteTestimonials(ctx, tx, siteID, in.Testimonials); err != nil {
		return nil, fmt.Errorf("save testimonials: %w", err)
	}
	if err := postgres.ReplaceSiteGalleryImages(ctx, tx, siteID, in.GalleryImages); err != nil {
		return nil, fmt.Errorf("save gallery: %w", err)
	}
	if err := postgres.ReplaceSiteBusinessHours(ctx, tx, siteID, in.BusinessHours); err != nil {
		return nil, fmt.Errorf("save business hours: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return s.GetSiteAggregate(ctx, siteID)
}

func (s *Sites) uniqueSlug(ctx context.Context, businessName string) (string, error) {
	base := toSlug(businessName)
	if base == "" {
		base = "site"
	}
	slug := base
	for i := 2; ; i++ {
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

// GetSiteAggregate loads a site and everything related to it.
func (s *Sites) GetSiteAggregate(ctx context.Context, id int) (*domain.SiteAggregate, error) {
	q := s.store.DB()

	site, err := postgres.GetSiteByID(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, nil
	}

	contact, err := postgres.GetSiteContact(ctx, q, id)
	if err != nil {
		return nil, err
	}
	billing, err := postgres.GetSiteBilling(ctx, q, id)
	if err != nil {
		return nil, err
	}
	if billing == nil {
		billing = &domain.SiteBilling{SiteID: id}
	}
	analytics, err := postgres.GetSiteAnalyticsSettings(ctx, q, id)
	if err != nil {
		return nil, err
	}
	notify, err := postgres.GetSiteNotifySettings(ctx, q, id)
	if err != nil {
		return nil, err
	}
	announcement, err := postgres.GetSiteAnnouncement(ctx, q, id)
	if err != nil {
		return nil, err
	}
	socialLinks, err := postgres.GetSiteSocialLinks(ctx, q, id)
	if err != nil {
		return nil, err
	}
	services, err := postgres.GetSiteServices(ctx, q, id)
	if err != nil {
		return nil, err
	}
	certifications, err := postgres.GetSiteCertifications(ctx, q, id)
	if err != nil {
		return nil, err
	}
	testimonials, err := postgres.GetSiteTestimonials(ctx, q, id)
	if err != nil {
		return nil, err
	}
	gallery, err := postgres.GetSiteGalleryImages(ctx, q, id)
	if err != nil {
		return nil, err
	}
	hours, err := postgres.GetSiteBusinessHours(ctx, q, id)
	if err != nil {
		return nil, err
	}

	return &domain.SiteAggregate{
		Site:           *site,
		Contact:        *contact,
		Billing:        *billing,
		Analytics:      *analytics,
		Notify:         *notify,
		Announcement:   *announcement,
		SocialLinks:    socialLinks,
		Services:       services,
		Certifications: certifications,
		Testimonials:   testimonials,
		GalleryImages:  gallery,
		BusinessHours:  hours,
	}, nil
}

// GetSiteAggregateBySlug is used by the public site renderer.
func (s *Sites) GetSiteAggregateBySlug(ctx context.Context, slug string) (*domain.SiteAggregate, error) {
	site, err := postgres.GetSiteBySlug(ctx, s.store.DB(), slug)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, nil
	}
	return s.GetSiteAggregate(ctx, site.ID)
}

// GetSiteAggregateByCustomDomain is used by the public site renderer to
// resolve a Pro site's connected custom domain.
func (s *Sites) GetSiteAggregateByCustomDomain(ctx context.Context, host string) (*domain.SiteAggregate, error) {
	site, err := postgres.GetSiteByCustomDomain(ctx, s.store.DB(), host)
	if err != nil {
		return nil, err
	}
	if site == nil {
		return nil, nil
	}
	return s.GetSiteAggregate(ctx, site.ID)
}

func (s *Sites) ListSitesByOwner(ctx context.Context, ownerID uuid.UUID) ([]domain.Site, error) {
	return postgres.ListSitesByOwner(ctx, s.store.DB(), ownerID)
}

// ListAllSites is used by the superadmin cross-account view.
func (s *Sites) ListAllSites(ctx context.Context) ([]domain.Site, error) {
	return postgres.ListAllSites(ctx, s.store.DB())
}

// ListLiveSites is used by the public sitemap.
func (s *Sites) ListLiveSites(ctx context.Context) ([]domain.Site, error) {
	return postgres.ListLiveSites(ctx, s.store.DB())
}

// UpdateContentInput is the full editable content form for an existing site.
type UpdateContentInput struct {
	SiteID         int
	BusinessName   string
	Tagline        string
	About          string
	LogoURL        string
	CTAText        string
	Contact        domain.SiteContact
	SocialLinks    []domain.SocialLink
	Services       []domain.Service
	Certifications []domain.Certification
	Testimonials   []domain.Testimonial
	GalleryImages  []domain.GalleryImage
	BusinessHours  []domain.BusinessHours
}

// UpdateContent saves every editable content field for a site in one transaction.
func (s *Sites) UpdateContent(ctx context.Context, in UpdateContentInput) error {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	site := &domain.Site{ID: in.SiteID, BusinessName: in.BusinessName, Tagline: in.Tagline, About: in.About, LogoURL: in.LogoURL, CTAText: in.CTAText}
	if err := postgres.UpdateSiteContent(ctx, tx, site); err != nil {
		return fmt.Errorf("update site: %w", err)
	}
	in.Contact.SiteID = in.SiteID
	if err := postgres.UpsertSiteContact(ctx, tx, &in.Contact); err != nil {
		return fmt.Errorf("save contact: %w", err)
	}
	if err := postgres.ReplaceSiteSocialLinks(ctx, tx, in.SiteID, in.SocialLinks); err != nil {
		return fmt.Errorf("save social links: %w", err)
	}
	if err := postgres.ReplaceSiteServices(ctx, tx, in.SiteID, in.Services); err != nil {
		return fmt.Errorf("save services: %w", err)
	}
	if err := postgres.ReplaceSiteCertifications(ctx, tx, in.SiteID, in.Certifications); err != nil {
		return fmt.Errorf("save certifications: %w", err)
	}
	if err := postgres.ReplaceSiteTestimonials(ctx, tx, in.SiteID, in.Testimonials); err != nil {
		return fmt.Errorf("save testimonials: %w", err)
	}
	if err := postgres.ReplaceSiteGalleryImages(ctx, tx, in.SiteID, in.GalleryImages); err != nil {
		return fmt.Errorf("save gallery: %w", err)
	}
	if err := postgres.ReplaceSiteBusinessHours(ctx, tx, in.SiteID, in.BusinessHours); err != nil {
		return fmt.Errorf("save business hours: %w", err)
	}

	return tx.Commit()
}

func (s *Sites) UpdateAppearance(ctx context.Context, siteID int, palette, headingFont string) error {
	return postgres.UpdateSiteAppearance(ctx, s.store.DB(), siteID, palette, headingFont)
}

// SwitchTemplate changes a site's design. The palette is reset (not carried
// over) since palette IDs are template-specific — a palette valid for the
// old template may not exist on the new one. Heading font is a
// template-agnostic choice, so it's left as-is.
func (s *Sites) SwitchTemplate(ctx context.Context, siteID int, templateID string) error {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	current, err := postgres.GetSiteByID(ctx, tx, siteID)
	if err != nil {
		return fmt.Errorf("load site: %w", err)
	}
	if current == nil {
		return fmt.Errorf("site %d not found", siteID)
	}
	if err := postgres.UpdateSiteTemplate(ctx, tx, siteID, templateID); err != nil {
		return fmt.Errorf("update template: %w", err)
	}
	if err := postgres.UpdateSiteAppearance(ctx, tx, siteID, "", current.HeadingFont); err != nil {
		return fmt.Errorf("reset palette: %w", err)
	}
	return tx.Commit()
}

// RenameSlug changes a site's subdomain, recording the old slug in
// slug_redirects (next to sites.slug, in one transaction) so links to it
// keep working via a 301 in serveSiteBySlug. Limited to once per day per
// site to stop slug squatting/churn.
func (s *Sites) RenameSlug(ctx context.Context, siteID int, newSlugRaw string) error {
	newSlug := toSlug(newSlugRaw)
	if newSlug == "" {
		return ErrSlugInvalid
	}
	if reservedSlugs[newSlug] {
		return ErrSlugReserved
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	current, err := postgres.GetSiteByID(ctx, tx, siteID)
	if err != nil {
		return fmt.Errorf("load site: %w", err)
	}
	if current == nil {
		return fmt.Errorf("site %d not found", siteID)
	}
	if current.Slug == newSlug {
		return nil
	}
	if current.SlugChangedAt != nil && time.Since(*current.SlugChangedAt) < slugRenameCooldown {
		return ErrSlugRateLimited
	}

	taken, err := postgres.SlugInUse(ctx, tx, newSlug)
	if err != nil {
		return fmt.Errorf("check slug: %w", err)
	}
	if taken {
		return ErrSlugTaken
	}

	if err := postgres.CreateSlugRedirect(ctx, tx, current.Slug, siteID); err != nil {
		return fmt.Errorf("save redirect: %w", err)
	}
	if err := postgres.RenameSiteSlug(ctx, tx, siteID, newSlug); err != nil {
		return fmt.Errorf("rename slug: %w", err)
	}
	return tx.Commit()
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

// UpdateAnnouncement sets or clears a site's temporary banner. An empty
// text clears it regardless of expiresAt.
func (s *Sites) UpdateAnnouncement(ctx context.Context, siteID int, text string, expiresAt *time.Time) error {
	return postgres.UpsertSiteAnnouncement(ctx, s.store.DB(), &domain.SiteAnnouncement{
		SiteID: siteID, Text: text, ExpiresAt: expiresAt,
	})
}

func (s *Sites) UpdateAnalyticsFrequency(ctx context.Context, siteID int, frequency string) error {
	return postgres.UpsertSiteAnalyticsSettings(ctx, s.store.DB(), &domain.SiteAnalyticsSettings{
		SiteID: siteID, AnalyticsFrequency: frequency,
	})
}

// UpdateNotifySettings sets a site's SMS lead alert opt-in. Enabling it
// requires a Pro plan (per-message cost) and a mobile number in E.164
// format; disabling always succeeds regardless of plan so a downgraded
// owner can still turn it off.
func (s *Sites) UpdateNotifySettings(ctx context.Context, siteID int, mobileNumber string, enabled bool) error {
	if enabled {
		billing, err := postgres.GetSiteBilling(ctx, s.store.DB(), siteID)
		if err != nil {
			return err
		}
		if billing == nil || billing.Plan != domain.PlanPro {
			return ErrNotifyNotPro
		}
		if !e164Re.MatchString(mobileNumber) {
			return ErrNotifyInvalidNumber
		}
	}
	return postgres.UpsertSiteNotifySettings(ctx, s.store.DB(), &domain.SiteNotifySettings{
		SiteID: siteID, MobileNumber: mobileNumber, SMSAlertsEnabled: enabled,
	})
}

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
	return postgres.SetSiteStatus(ctx, s.store.DB(), siteID, domain.SiteStatusLive)
}

func (s *Sites) Unpublish(ctx context.Context, siteID int) error {
	return postgres.SetSiteStatus(ctx, s.store.DB(), siteID, domain.SiteStatusDraft)
}

// Delete removes a site and, if it had an active paid subscription,
// cancels it in Stripe first — otherwise the customer keeps being billed
// for a site that no longer exists, with no dashboard page left to cancel
// it from themselves.
func (s *Sites) Delete(ctx context.Context, siteID int) error {
	if err := s.billing.CancelSubscriptionIfActive(ctx, siteID); err != nil {
		return fmt.Errorf("cancel subscription: %w", err)
	}
	return postgres.DeleteSite(ctx, s.store.DB(), siteID)
}
