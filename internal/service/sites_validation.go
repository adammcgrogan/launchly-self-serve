package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

var hexColorRe = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

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

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func checkEmail(field, value string) error {
	if value == "" || emailRe.MatchString(value) {
		return nil
	}
	return &ValidationError{Message: fmt.Sprintf("enter a valid %s.", field), Field: field}
}

var phoneRe = regexp.MustCompile(`^[0-9+()\-.\s]{7,25}$`)

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
		checkURL("map embed URL", contact.MapEmbedURL),
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

// validateVideoURL checks an optional promo video link — empty is allowed,
// otherwise it must be a well-formed https URL that resolves to a YouTube or
// Vimeo video (checked via domain.Site.VideoEmbedURL so the accepted formats
// stay in one place).
func validateVideoURL(videoURL string) error {
	if videoURL == "" {
		return nil
	}
	if err := checkLen("video URL", videoURL, maxMediumField); err != nil {
		return err
	}
	if err := checkURL("video URL", videoURL); err != nil {
		return err
	}
	if (domain.Site{VideoURL: videoURL}).VideoEmbedURL() == "" {
		return &ValidationError{Message: "video URL must be a YouTube or Vimeo link.", Field: "video URL"}
	}
	return nil
}

// validateDocument checks an optional downloadable document (menu/brochure
// PDF link) — both fields are independently optional, but a title is
// meaningless without a URL to attach it to.
func validateDocument(title, documentURL string) error {
	if err := checkLen("document title", title, maxShortField); err != nil {
		return err
	}
	if err := checkLen("document URL", documentURL, maxMediumField); err != nil {
		return err
	}
	if err := checkURL("document URL", documentURL); err != nil {
		return err
	}
	if documentURL == "" && title != "" {
		return &ValidationError{Message: "add a document file before setting a title.", Field: "document title"}
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

// validateBusinessHours rejects a row where OpensAt equals ClosesAt: that's
// almost always a copy-paste mistake, not a genuine "open 24 hours" span, and
// domain.SiteAggregate.OpenNow treats ClosesAt <= OpensAt as an overnight
// span (e.g. 18:00-02:00), so an equal pair would otherwise be silently
// misread as open all day and into the night.
func validateBusinessHours(hours []domain.BusinessHours) error {
	for _, h := range hours {
		if h.Closed || h.OpensAt == "" || h.ClosesAt == "" {
			continue
		}
		if h.OpensAt == h.ClosesAt {
			return &ValidationError{Message: "opening and closing time can't be the same — mark the day as closed instead.", Field: "business hours"}
		}
	}
	return nil
}
