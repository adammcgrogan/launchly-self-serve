package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// maxCopyInputLen bounds the business/service name forwarded into the Gemini
// prompt, matching service.validateSiteContent's maxShortField cap on the
// same owner-entered fields.
const maxCopyInputLen = 200

// wizardStepForField maps a ValidationError's Field (the canonical name
// passed to checkLen/checkURL/checkEmail/checkPhone in
// internal/service/sites.go) to the new-site wizard step that contains it,
// so a failed submit can return the owner to the field that's actually
// wrong instead of resetting to step 1. Fields not present in the wizard
// (e.g. FAQ/staff/meta fields, which only exist on the edit-site page) fall
// through to step 1.
func wizardStepForField(field string) int {
	switch field {
	case "business name", "location":
		return 1
	case "tagline", "about", "contact phone", "contact email", "service", "service price", "service description":
		return 3
	case "CTA text", "logo URL", "address", "map embed URL", "certification",
		"testimonial author name", "testimonial author role", "testimonial quote",
		"gallery image URL", "gallery image alt text",
		"facebook link", "instagram link", "whatsapp link", "twitter link", "tiktok link", "linkedin link", "youtube link":
		return 4
	default:
		return 1
	}
}

// renderNewSite renders the builder wizard. values carries the user's input
// back across a failed submit so nothing they typed is lost. errStep is the
// wizard step the JS should land on — 1 unless a ValidationError pinpointed
// a field on a later step.
func (h *Handler) renderNewSite(w http.ResponseWriter, r *http.Request, errMsg string, errStep int, values url.Values) {
	if values == nil {
		values = url.Values{}
	}
	if errStep == 0 {
		errStep = 1
	}
	h.render.Render(w, "dashboard:new_site", map[string]any{
		"Templates":        siteTemplates,
		"BusinessTypes":    businessTypes,
		"PaletteColors":    paletteSwatchColors,
		"Error":            errMsg,
		"ErrorStep":        errStep,
		"Values":           values,
		"Weekdays":         weekdays,
		"Timezones":        timezones,
		"TestimonialRows":  testimonialRowsForForm(values),
		"ServiceRows":      serviceRowsForForm(values),
		"CSRFToken":        h.csrf.Token(middleware.UserID(r).String(), h.auth.SessionNonce(r)),
		"EmailVerified":    h.emailVerified(r),
		"AIAvailable":      h.ai.Configured(),
		"UploadsAvailable": h.uploads.Available(),
	})
}

// NewSiteForm renders the builder wizard: a four-step flow (business
// basics, design, contact, optional extras) that ends in an instant publish.
func (h *Handler) NewSiteForm(w http.ResponseWriter, r *http.Request) {
	h.renderNewSite(w, r, "", 0, nil)
}

// NewSiteSubmit creates the site and publishes it immediately — there is no
// draft/review step, so the customer's site is live the moment they submit.
func (h *Handler) NewSiteSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	businessName := strings.TrimSpace(r.FormValue("business_name"))
	templateID := r.FormValue("template_id")
	if businessName == "" {
		h.renderNewSite(w, r, "Business name is required.", 1, r.Form)
		return
	}
	tmpl, ok := findTemplate(templateID)
	if !ok {
		tmpl = siteTemplates[0]
		templateID = tmpl.ID
	}

	palette := r.FormValue("palette")
	paletteValid := palette == ""
	for _, p := range tmpl.Palettes {
		if p.ID == palette {
			paletteValid = true
			break
		}
	}
	if !paletteValid {
		palette = ""
	}
	headingFont := r.FormValue("heading_font")
	if headingFont != "sans" && headingFont != "serif" {
		headingFont = ""
	}

	in := service.CreateSiteInput{
		OwnerUserID:  middleware.UserID(r),
		BusinessName: businessName,
		Tagline:      strings.TrimSpace(r.FormValue("tagline")),
		About:        strings.TrimSpace(r.FormValue("about")),
		LogoURL:      strings.TrimSpace(r.FormValue("logo_url")),
		CTAText:      strings.TrimSpace(r.FormValue("cta_text")),
		TemplateID:   templateID,
		Palette:      palette,
		HeadingFont:  headingFont,
		Timezone:     resolveTimezone(r.FormValue("timezone")),
		Contact: domain.SiteContact{
			Phone:       strings.TrimSpace(r.FormValue("phone")),
			Email:       strings.TrimSpace(r.FormValue("email")),
			Address:     strings.TrimSpace(r.FormValue("address")),
			Location:    strings.TrimSpace(r.FormValue("location")),
			MapEmbedURL: strings.TrimSpace(r.FormValue("map_embed_url")),
		},
		SocialLinks:    parseSocialLinks(r),
		Services:       parseServiceRows(r),
		Certifications: parseCertifications(r.FormValue("certifications")),
		Testimonials:   parseTestimonialRows(r),
		GalleryImages:  parseGallery(r.FormValue("gallery")),
		BusinessHours:  parseBusinessHours(r),
	}

	site, err := h.sites.CreateSite(r.Context(), in)
	if err != nil {
		var verr *service.ValidationError
		if errors.As(err, &verr) {
			h.renderNewSite(w, r, verr.Message, wizardStepForField(verr.Field), r.Form)
			return
		}
		if errors.Is(err, service.ErrSiteLimitReached) {
			h.renderNewSite(w, r, service.ErrSiteLimitReached.Error(), 1, r.Form)
			return
		}
		h.renderNewSite(w, r, "Something went wrong creating your site. Please try again.", 1, r.Form)
		return
	}

	http.Redirect(w, r, "/dashboard/sites/"+site.Slug+"?launched=1", http.StatusSeeOther)
}

// GenerateCopy drafts a single content field — tagline, about text, or one
// service's description — from a business name + type for the "Generate"
// buttons next to those fields in the builder wizard. The owner reviews and
// can edit the draft before it's ever saved — this endpoint never writes to
// the database.
func (h *Handler) GenerateCopy(w http.ResponseWriter, r *http.Request) {
	if !h.ai.Configured() {
		http.Error(w, "not available", http.StatusServiceUnavailable)
		return
	}
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if !h.aiGenerateLimiter.Allow(middleware.UserID(r).String()) {
		http.Error(w, "too many requests, try again shortly", http.StatusTooManyRequests)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)

	businessName := strings.TrimSpace(r.FormValue("business_name"))
	if businessName == "" {
		http.Error(w, "business name is required", http.StatusBadRequest)
		return
	}
	// Cap prompt inputs like validateSiteContent caps owner content
	// (maxShortField), so a single request can't balloon the Gemini prompt.
	if len(businessName) > maxCopyInputLen {
		http.Error(w, "business name is too long", http.StatusBadRequest)
		return
	}
	businessType := "general business or trade"
	for _, bt := range businessTypes {
		if bt.ID == r.FormValue("business_type") {
			businessType = bt.Label
			break
		}
	}

	var text string
	var err error
	switch r.FormValue("field") {
	case "tagline":
		text, err = h.ai.GenerateTagline(r.Context(), businessName, businessType)
	case "about":
		text, err = h.ai.GenerateAbout(r.Context(), businessName, businessType)
	case "service_description":
		serviceName := strings.TrimSpace(r.FormValue("service_name"))
		if serviceName == "" {
			http.Error(w, "service name is required", http.StatusBadRequest)
			return
		}
		if len(serviceName) > maxCopyInputLen {
			http.Error(w, "service name is too long", http.StatusBadRequest)
			return
		}
		text, err = h.ai.GenerateServiceDescription(r.Context(), businessName, businessType, serviceName)
	default:
		http.Error(w, "unknown field", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, "couldn't generate content, please try again", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"text": text})
}
