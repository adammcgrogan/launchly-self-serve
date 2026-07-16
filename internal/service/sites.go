package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/sync/errgroup"
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
	emailRe     = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
	phoneRe     = regexp.MustCompile(`^[0-9+()\-.\s]{7,25}$`)
	hexColorRe  = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// IsValidHexColor reports whether s is a 6-digit hex colour in "#RRGGBB"
// form, the format brand-colour input is normalized to before saving.
func IsValidHexColor(s string) bool {
	return hexColorRe.MatchString(s)
}

// Server-side max lengths for site content fields, generous enough for real
// business content but bounded so a pasted wall of text can't bloat a row or
// break layout on the public site page.
const (
	maxShortField  = 200  // names, labels, single-line fields
	maxMediumField = 500  // taglines, addresses, URLs
	maxLongField   = 5000 // about text, testimonial quotes
)

// ValidationError is returned by CreateSite/UpdateContent when submitted
// content fails format or length validation. Message is safe to show the
// user directly. Field is the canonical field name passed to checkLen/
// checkURL/checkEmail/checkPhone, so callers can map a failure back to the
// form field or wizard step it came from.
type ValidationError struct {
	Message string
	Field   string
}

func (e *ValidationError) Error() string { return e.Message }

func checkLen(field, value string, max int) error {
	if utf8.RuneCountInString(value) > max {
		return &ValidationError{Message: fmt.Sprintf("%s is too long (max %d characters).", field, max), Field: field}
	}
	return nil
}

// checkURL requires an absolute https URL — empty is allowed since these
// fields are optional. http is rejected outright: every published site is
// served over https, so an http:// asset URL is silently blocked by the
// browser as mixed content.
func checkURL(field, value string) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Host == "" || u.Scheme != "https" {
		return &ValidationError{Message: fmt.Sprintf("enter a valid %s starting with https://.", field), Field: field}
	}
	return nil
}

func checkEmail(field, value string) error {
	if value == "" || emailRe.MatchString(value) {
		return nil
	}
	return &ValidationError{Message: fmt.Sprintf("enter a valid %s.", field), Field: field}
}

func checkPhone(field, value string) error {
	if value == "" || phoneRe.MatchString(value) {
		return nil
	}
	return &ValidationError{Message: fmt.Sprintf("enter a valid %s.", field), Field: field}
}

// validateSiteContent checks format (email/phone/logo/map/gallery URLs) and
// length limits across every editable content field, shared by CreateSite
// and UpdateContent so the builder and editor enforce the same rules.
func validateSiteContent(businessName, tagline, about, logoURL, ctaText string, contact domain.SiteContact, social []domain.SocialLink, services []domain.Service, certs []domain.Certification, testimonials []domain.Testimonial, gallery []domain.GalleryImage, faqItems []domain.FAQItem, staff []domain.StaffMember, areas []domain.ServiceArea) error {
	checks := []error{
		checkLen("business name", businessName, maxShortField),
		checkLen("tagline", tagline, maxMediumField),
		checkLen("about", about, maxLongField),
		checkLen("CTA text", ctaText, maxShortField),
		checkLen("logo URL", logoURL, maxMediumField),
		checkURL("logo URL", logoURL),
		checkEmail("contact email", contact.Email),
		checkPhone("contact phone", contact.Phone),
		checkLen("address", contact.Address, maxMediumField),
		checkLen("location", contact.Location, maxShortField),
		checkLen("map URL", contact.MapURL, maxMediumField),
		checkURL("map URL", contact.MapURL),
		checkLen("map embed URL", contact.MapEmbedURL, maxMediumField),
	}
	for _, sl := range social {
		checks = append(checks, checkLen(string(sl.Platform)+" link", sl.URL, maxMediumField))
	}
	for _, sv := range services {
		checks = append(checks,
			checkLen("service", sv.Label, maxShortField),
			checkLen("service price", sv.PriceText, maxShortField),
			checkLen("service description", sv.Description, maxMediumField),
		)
	}
	for _, c := range certs {
		checks = append(checks, checkLen("certification", c.Label, maxShortField))
	}
	for _, t := range testimonials {
		checks = append(checks,
			checkLen("testimonial author name", t.AuthorName, maxShortField),
			checkLen("testimonial author role", t.AuthorRole, maxShortField),
			checkLen("testimonial quote", t.Quote, maxLongField),
		)
	}
	for _, g := range gallery {
		checks = append(checks,
			checkLen("gallery image URL", g.URL, maxMediumField),
			checkURL("gallery image URL", g.URL),
			checkLen("gallery image alt text", g.AltText, maxMediumField),
		)
	}
	for _, f := range faqItems {
		checks = append(checks,
			checkLen("FAQ question", f.Question, maxMediumField),
			checkLen("FAQ answer", f.Answer, maxLongField),
		)
	}
	for _, m := range staff {
		checks = append(checks,
			checkLen("staff name", m.Name, maxShortField),
			checkLen("staff role", m.Role, maxShortField),
			checkLen("staff photo URL", m.PhotoURL, maxMediumField),
			checkURL("staff photo URL", m.PhotoURL),
			checkLen("staff bio", m.Bio, maxLongField),
		)
	}
	for _, a := range areas {
		checks = append(checks, checkLen("service area", a.Area, maxShortField))
	}
	for _, err := range checks {
		if err != nil {
			return err
		}
	}
	return nil
}

// validateSEO checks the optional per-site SEO overrides — meta title/
// description length and that the OG image is a valid https URL.
func validateSEO(metaTitle, metaDescription, ogImageURL string) error {
	for _, err := range []error{
		checkLen("meta title", metaTitle, maxMediumField),
		checkLen("meta description", metaDescription, maxMediumField),
		checkLen("share image URL", ogImageURL, maxMediumField),
		checkURL("share image URL", ogImageURL),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

// validateReviews checks the owner-entered review rating badge: the rating
// must be a number between 0 and 5, the count non-negative, and the review
// link a valid https URL. All fields are optional (empty rating = no badge).
func validateReviews(r domain.SiteReviews) error {
	if r.Rating != "" {
		v, err := strconv.ParseFloat(r.Rating, 64)
		if err != nil || v < 0 || v > 5 {
			return &ValidationError{Message: "review rating must be a number between 0 and 5."}
		}
	}
	if r.ReviewCount < 0 {
		return &ValidationError{Message: "review count can't be negative."}
	}
	if err := checkLen("review link", r.ReviewURL, maxMediumField); err != nil {
		return err
	}
	return checkURL("review link", r.ReviewURL)
}

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

// Errors returned by UpdateNotifySettings — web handlers show these
// directly to the site owner, so their text is user-facing.
var (
	ErrNotifyNotPro        = errors.New("SMS lead alerts are a Pro feature.")
	ErrNotifyInvalidNumber = errors.New("enter your mobile number in international format, e.g. +447700900123.")
)

// Errors returned by UpdateTrackingSettings — shown directly to the owner.
var (
	ErrTrackingNotPro  = errors.New("your own analytics tracking is a Pro feature.")
	ErrTrackingInvalid = errors.New("check your GA4 measurement ID (G-XXXXXXXXXX) and Meta Pixel ID (numeric).")
)

var (
	ga4IDRe   = regexp.MustCompile(`^G-[A-Z0-9]{4,20}$`)
	pixelIDRe = regexp.MustCompile(`^[0-9]{5,20}$`)
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
// in one transaction, and sets it live immediately with a 14-day trial —
// there is no draft/review step. uniqueSlug's read happens outside the
// insert transaction, so two concurrent creates for the same business name
// can both pick the same slug; if the insert then loses that race on the
// slug's unique constraint, we regenerate and retry rather than surfacing a
// 500.
func (s *Sites) CreateSite(ctx context.Context, in CreateSiteInput) (*domain.SiteAggregate, error) {
	if err := validateSiteContent(in.BusinessName, in.Tagline, in.About, in.LogoURL, in.CTAText, in.Contact, in.SocialLinks, in.Services, in.Certifications, in.Testimonials, in.GalleryImages, in.FAQItems, in.StaffMembers, nil); err != nil {
		return nil, err
	}

	allowed, err := s.canCreateSite(ctx, in.OwnerUserID)
	if err != nil {
		return nil, fmt.Errorf("check site limit: %w", err)
	}
	if !allowed {
		return nil, ErrSiteLimitReached
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
// lifts the cap, since plan is tracked per site rather than per account.
func (s *Sites) canCreateSite(ctx context.Context, ownerID uuid.UUID) (bool, error) {
	count, err := postgres.CountSitesByOwner(ctx, s.store.DB(), ownerID)
	if err != nil {
		return false, err
	}
	if count == 0 {
		return true, nil
	}
	return postgres.OwnerHasProSite(ctx, s.store.DB(), ownerID)
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

	var (
		contact        *domain.SiteContact
		billing        *domain.SiteBilling
		analytics      *domain.SiteAnalyticsSettings
		notify         *domain.SiteNotifySettings
		announcement   *domain.SiteAnnouncement
		reviews        *domain.SiteReviews
		socialLinks    []domain.SocialLink
		services       []domain.Service
		certifications []domain.Certification
		testimonials   []domain.Testimonial
		gallery        []domain.GalleryImage
		faqItems       []domain.FAQItem
		staffMembers   []domain.StaffMember
		hours          []domain.BusinessHours
		specialHours   []domain.SpecialHours
		serviceAreas   []domain.ServiceArea
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() (err error) { contact, err = postgres.GetSiteContact(gctx, q, id); return })
	g.Go(func() (err error) { billing, err = postgres.GetSiteBilling(gctx, q, id); return })
	g.Go(func() (err error) { analytics, err = postgres.GetSiteAnalyticsSettings(gctx, q, id); return })
	g.Go(func() (err error) { notify, err = postgres.GetSiteNotifySettings(gctx, q, id); return })
	g.Go(func() (err error) { announcement, err = postgres.GetSiteAnnouncement(gctx, q, id); return })
	g.Go(func() (err error) { reviews, err = postgres.GetSiteReviews(gctx, q, id); return })
	g.Go(func() (err error) { socialLinks, err = postgres.GetSiteSocialLinks(gctx, q, id); return })
	g.Go(func() (err error) { services, err = postgres.GetSiteServices(gctx, q, id); return })
	g.Go(func() (err error) { certifications, err = postgres.GetSiteCertifications(gctx, q, id); return })
	g.Go(func() (err error) { testimonials, err = postgres.GetSiteTestimonials(gctx, q, id); return })
	g.Go(func() (err error) { gallery, err = postgres.GetSiteGalleryImages(gctx, q, id); return })
	g.Go(func() (err error) { faqItems, err = postgres.GetSiteFAQItems(gctx, q, id); return })
	g.Go(func() (err error) { staffMembers, err = postgres.GetSiteStaffMembers(gctx, q, id); return })
	g.Go(func() (err error) { hours, err = postgres.GetSiteBusinessHours(gctx, q, id); return })
	g.Go(func() (err error) { specialHours, err = postgres.GetSiteSpecialHours(gctx, q, id); return })
	g.Go(func() (err error) { serviceAreas, err = postgres.GetSiteServiceAreas(gctx, q, id); return })
	if err := g.Wait(); err != nil {
		return nil, err
	}

	if billing == nil {
		billing = &domain.SiteBilling{SiteID: id}
	}

	return &domain.SiteAggregate{
		Site:           *site,
		Contact:        *contact,
		Billing:        *billing,
		Analytics:      *analytics,
		Notify:         *notify,
		Announcement:   *announcement,
		Reviews:        *reviews,
		SocialLinks:    socialLinks,
		Services:       services,
		Certifications: certifications,
		Testimonials:   testimonials,
		GalleryImages:  gallery,
		FAQItems:       faqItems,
		StaffMembers:   staffMembers,
		BusinessHours:  hours,
		SpecialHours:   specialHours,
		ServiceAreas:   serviceAreas,
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
	SiteID          int
	BusinessName    string
	Tagline         string
	About           string
	LogoURL         string
	CTAText         string
	Timezone        string
	MetaTitle       string
	MetaDescription string
	OgImageURL      string
	Contact         domain.SiteContact
	SocialLinks     []domain.SocialLink
	Services        []domain.Service
	Certifications  []domain.Certification
	Testimonials    []domain.Testimonial
	GalleryImages   []domain.GalleryImage
	FAQItems        []domain.FAQItem
	StaffMembers    []domain.StaffMember
	BusinessHours   []domain.BusinessHours
	SpecialHours    []domain.SpecialHours
	ServiceAreas    []domain.ServiceArea
	Reviews         domain.SiteReviews
}

// UpdateContent saves every editable content field for a site in one transaction.
func (s *Sites) UpdateContent(ctx context.Context, in UpdateContentInput) error {
	if err := validateSiteContent(in.BusinessName, in.Tagline, in.About, in.LogoURL, in.CTAText, in.Contact, in.SocialLinks, in.Services, in.Certifications, in.Testimonials, in.GalleryImages, in.FAQItems, in.StaffMembers, in.ServiceAreas); err != nil {
		return err
	}
	if err := validateReviews(in.Reviews); err != nil {
		return err
	}
	if err := validateSEO(in.MetaTitle, in.MetaDescription, in.OgImageURL); err != nil {
		return err
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	site := &domain.Site{ID: in.SiteID, BusinessName: in.BusinessName, Tagline: in.Tagline, About: in.About, LogoURL: in.LogoURL, CTAText: in.CTAText, Timezone: in.Timezone,
		MetaTitle: in.MetaTitle, MetaDescription: in.MetaDescription, OgImageURL: in.OgImageURL}
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
	if err := postgres.ReplaceSiteFAQItems(ctx, tx, in.SiteID, in.FAQItems); err != nil {
		return fmt.Errorf("save FAQ items: %w", err)
	}
	if err := postgres.ReplaceSiteStaffMembers(ctx, tx, in.SiteID, in.StaffMembers); err != nil {
		return fmt.Errorf("save staff members: %w", err)
	}
	if err := postgres.ReplaceSiteBusinessHours(ctx, tx, in.SiteID, in.BusinessHours); err != nil {
		return fmt.Errorf("save business hours: %w", err)
	}
	if err := postgres.ReplaceSiteSpecialHours(ctx, tx, in.SiteID, in.SpecialHours); err != nil {
		return fmt.Errorf("save special hours: %w", err)
	}
	if err := postgres.ReplaceSiteServiceAreas(ctx, tx, in.SiteID, in.ServiceAreas); err != nil {
		return fmt.Errorf("save service areas: %w", err)
	}
	in.Reviews.SiteID = in.SiteID
	if err := postgres.UpsertSiteReviews(ctx, tx, &in.Reviews); err != nil {
		return fmt.Errorf("save reviews: %w", err)
	}

	return tx.Commit()
}

func (s *Sites) UpdateAppearance(ctx context.Context, siteID int, palette, headingFont, brandColor string) error {
	return postgres.UpdateSiteAppearance(ctx, s.store.DB(), siteID, palette, headingFont, brandColor)
}

// UpdateFormType switches a site's public form between the plain contact
// form and the booking form (service + preferred time).
func (s *Sites) UpdateFormType(ctx context.Context, siteID int, formType domain.FormType) error {
	return postgres.UpdateSiteFormType(ctx, s.store.DB(), siteID, formType)
}

// SwitchTemplate changes a site's design. The palette is reset (not carried
// over) since palette IDs are template-specific — a palette valid for the
// old template may not exist on the new one. Heading font and brand colour
// are template-agnostic choices, so they're left as-is.
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
	if err := postgres.UpdateSiteAppearance(ctx, tx, siteID, "", current.HeadingFont, current.BrandColor); err != nil {
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
func (s *Sites) UpdateAnnouncement(ctx context.Context, siteID int, text string, expiresAt *time.Time, tone domain.AnnouncementTone, linkURL, linkLabel string) error {
	return postgres.UpsertSiteAnnouncement(ctx, s.store.DB(), &domain.SiteAnnouncement{
		SiteID: siteID, Text: text, ExpiresAt: expiresAt, Tone: tone, LinkURL: linkURL, LinkLabel: linkLabel,
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

// UpdateTrackingSettings saves a Pro site's own GA4 measurement ID and Meta
// Pixel ID, validating each ID's format. Both are optional; clearing them
// (empty strings) always succeeds regardless of plan so a downgraded owner
// can still remove their tags.
func (s *Sites) UpdateTrackingSettings(ctx context.Context, siteID int, gaMeasurementID, metaPixelID string) error {
	gaMeasurementID = strings.ToUpper(strings.TrimSpace(gaMeasurementID))
	metaPixelID = strings.TrimSpace(metaPixelID)

	if gaMeasurementID != "" || metaPixelID != "" {
		billing, err := postgres.GetSiteBilling(ctx, s.store.DB(), siteID)
		if err != nil {
			return err
		}
		if billing == nil || billing.Plan != domain.PlanPro {
			return ErrTrackingNotPro
		}
		if gaMeasurementID != "" && !ga4IDRe.MatchString(gaMeasurementID) {
			return ErrTrackingInvalid
		}
		if metaPixelID != "" && !pixelIDRe.MatchString(metaPixelID) {
			return ErrTrackingInvalid
		}
	}
	return postgres.UpsertSiteTrackingIDs(ctx, s.store.DB(), siteID, gaMeasurementID, metaPixelID)
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
	if err := postgres.DeleteSite(ctx, s.store.DB(), siteID); err != nil {
		return err
	}
	slog.Info("site deleted", "site_id", siteID)
	return nil
}
