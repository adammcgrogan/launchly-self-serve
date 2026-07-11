package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

// Sites owns the create/edit/publish lifecycle of a site and assembling the
// full SiteAggregate from the many tables it spans.
type Sites struct {
	store *postgres.Store
}

func NewSites(store *postgres.Store) *Sites {
	return &Sites{store: store}
}

var (
	slugStripRe = regexp.MustCompile(`['\x60]`)
	slugCharsRe = regexp.MustCompile(`[^a-z0-9]+`)
)

func toSlug(s string) string {
	s = strings.ToLower(s)
	s = slugStripRe.ReplaceAllString(s, "")
	s = slugCharsRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
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

func (s *Sites) UpdateAnalyticsFrequency(ctx context.Context, siteID int, frequency string) error {
	return postgres.UpsertSiteAnalyticsSettings(ctx, s.store.DB(), &domain.SiteAnalyticsSettings{
		SiteID: siteID, AnalyticsFrequency: frequency,
	})
}

// Publish and Unpublish let an owner take their own site up/down at will —
// there is no admin approval gate on either direction.
func (s *Sites) Publish(ctx context.Context, siteID int) error {
	return postgres.SetSiteStatus(ctx, s.store.DB(), siteID, domain.SiteStatusLive)
}

func (s *Sites) Unpublish(ctx context.Context, siteID int) error {
	return postgres.SetSiteStatus(ctx, s.store.DB(), siteID, domain.SiteStatusDraft)
}

func (s *Sites) Delete(ctx context.Context, siteID int) error {
	return postgres.DeleteSite(ctx, s.store.DB(), siteID)
}
