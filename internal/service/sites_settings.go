package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
)

// UpdateContentInput is the full editable content form for an existing site.
type UpdateContentInput struct {
	SiteID          int
	BusinessName    string
	Tagline         string
	About           string
	LogoURL         string
	CTAText         string
	VideoURL        string
	ThankYouMessage string
	RedirectURL     string
	Timezone        string
	MetaTitle       string
	MetaDescription string
	OgImageURL      string
	DocumentTitle   string
	DocumentURL     string
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
	if err := validateVideoURL(in.VideoURL); err != nil {
		return err
	}
	if err := validateBusinessHours(in.BusinessHours); err != nil {
		return err
	}
	if err := checkLen("thank-you message", in.ThankYouMessage, maxMediumField); err != nil {
		return err
	}
	if err := checkLen("redirect URL", in.RedirectURL, maxMediumField); err != nil {
		return err
	}
	if err := checkURL("redirect URL", in.RedirectURL); err != nil {
		return err
	}
	if err := validateDocument(in.DocumentTitle, in.DocumentURL); err != nil {
		return err
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	prevSite, err := postgres.GetSiteByID(ctx, tx, in.SiteID)
	if err != nil {
		return fmt.Errorf("load site: %w", err)
	}
	prevGallery, err := postgres.GetSiteGalleryImages(ctx, tx, in.SiteID)
	if err != nil {
		return fmt.Errorf("load gallery: %w", err)
	}

	site := &domain.Site{ID: in.SiteID, BusinessName: in.BusinessName, Tagline: in.Tagline, About: in.About, LogoURL: in.LogoURL, CTAText: in.CTAText, VideoURL: in.VideoURL, Timezone: in.Timezone,
		MetaTitle: in.MetaTitle, MetaDescription: in.MetaDescription, OgImageURL: in.OgImageURL,
		ThankYouMessage: in.ThankYouMessage, RedirectURL: in.RedirectURL,
		DocumentTitle: in.DocumentTitle, DocumentURL: in.DocumentURL}
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

	if err := tx.Commit(); err != nil {
		return err
	}

	s.invalidateAggregate(in.SiteID)
	s.deleteStaleImages(ctx, prevSite, prevGallery, in)
	return nil
}

// deleteStaleImages removes the previous logo/gallery blobs from Storage
// once the content update that replaced or removed them has committed.
// Best-effort: the content save has already succeeded, so a Storage error
// here is logged, not surfaced — a missed cleanup just leaves an orphaned
// object rather than losing the owner's edit.
func (s *Sites) deleteStaleImages(ctx context.Context, prevSite *domain.Site, prevGallery []domain.GalleryImage, in UpdateContentInput) {
	if s.uploads == nil {
		return
	}
	if prevSite != nil && prevSite.LogoURL != "" && prevSite.LogoURL != in.LogoURL {
		if err := s.uploads.DeleteImage(ctx, prevSite.LogoURL); err != nil {
			slog.Error("delete stale logo image", "site_id", in.SiteID, "error", err)
		}
	}
	if prevSite != nil && prevSite.DocumentURL != "" && prevSite.DocumentURL != in.DocumentURL {
		if err := s.uploads.DeleteImage(ctx, prevSite.DocumentURL); err != nil {
			slog.Error("delete stale document", "site_id", in.SiteID, "error", err)
		}
	}
	kept := make(map[string]bool, len(in.GalleryImages))
	for _, img := range in.GalleryImages {
		kept[img.URL] = true
	}
	for _, img := range prevGallery {
		if !kept[img.URL] {
			if err := s.uploads.DeleteImage(ctx, img.URL); err != nil {
				slog.Error("delete stale gallery image", "site_id", in.SiteID, "error", err)
			}
		}
	}
}

func (s *Sites) UpdateAppearance(ctx context.Context, siteID int, palette, headingFont, brandColor string) error {
	err := postgres.UpdateSiteAppearance(ctx, s.store.DB(), siteID, palette, headingFont, brandColor)
	if err == nil {
		s.invalidateAggregate(siteID)
	}
	return err
}

// UpdateFormType switches a site's public form between the plain contact
// form and the booking form (service + preferred time).
func (s *Sites) UpdateFormType(ctx context.Context, siteID int, formType domain.FormType) error {
	err := postgres.UpdateSiteFormType(ctx, s.store.DB(), siteID, formType)
	if err == nil {
		s.invalidateAggregate(siteID)
	}
	return err
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
	if err := tx.Commit(); err != nil {
		return err
	}
	s.invalidateAggregate(siteID)
	return nil
}

// UpdateAnnouncement sets or clears a site's temporary banner. An empty
// text clears it regardless of expiresAt.
func (s *Sites) UpdateAnnouncement(ctx context.Context, siteID int, text string, expiresAt *time.Time, tone domain.AnnouncementTone, linkURL, linkLabel string) error {
	err := postgres.UpsertSiteAnnouncement(ctx, s.store.DB(), &domain.SiteAnnouncement{
		SiteID: siteID, Text: text, ExpiresAt: expiresAt, Tone: tone, LinkURL: linkURL, LinkLabel: linkLabel,
	})
	if err == nil {
		s.invalidateAggregate(siteID)
	}
	return err
}

func (s *Sites) UpdateAnalyticsFrequency(ctx context.Context, siteID int, frequency string) error {
	err := postgres.UpsertSiteAnalyticsSettings(ctx, s.store.DB(), &domain.SiteAnalyticsSettings{
		SiteID: siteID, AnalyticsFrequency: frequency,
	})
	if err == nil {
		s.invalidateAggregate(siteID)
	}
	return err
}

// Errors returned by UpdateNotifySettings — web handlers show these
// directly to the site owner, so their text is user-facing.
var (
	ErrNotifyNotPro        = errors.New("SMS lead alerts are a Pro feature.")
	ErrNotifyInvalidNumber = errors.New("enter your mobile number in international format, e.g. +447700900123.")
)

var e164Re = regexp.MustCompile(`^\+[1-9]\d{6,14}$`)

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
		if billing == nil || !billing.IsPro() {
			return ErrNotifyNotPro
		}
		if !e164Re.MatchString(mobileNumber) {
			return ErrNotifyInvalidNumber
		}
	}
	err := postgres.UpsertSiteNotifySettings(ctx, s.store.DB(), &domain.SiteNotifySettings{
		SiteID: siteID, MobileNumber: mobileNumber, SMSAlertsEnabled: enabled,
	})
	if err == nil {
		s.invalidateAggregate(siteID)
	}
	return err
}
