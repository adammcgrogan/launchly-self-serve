package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// renderNewSite renders the builder wizard. values carries the user's input
// back across a failed submit so nothing they typed is lost.
func (h *Handler) renderNewSite(w http.ResponseWriter, r *http.Request, errMsg string, values url.Values) {
	if values == nil {
		values = url.Values{}
	}
	h.render.Render(w, "dashboard:new_site", map[string]any{
		"Templates":       siteTemplates,
		"BusinessTypes":   businessTypes,
		"PaletteColors":   paletteSwatchColors,
		"Error":           errMsg,
		"Values":          values,
		"Weekdays":        weekdays,
		"Timezones":       timezones,
		"TestimonialRows": testimonialRowsForForm(values),
		"ServiceRows":     serviceRowsForForm(values),
		"CSRFToken":       h.csrf.Token(middleware.UserID(r).String()),
		"EmailVerified":   h.emailVerified(r),
		"AIAvailable":     h.ai.Configured(),
	})
}

// NewSiteForm renders the builder wizard: a four-step flow (business
// basics, design, contact, optional extras) that ends in an instant publish.
func (h *Handler) NewSiteForm(w http.ResponseWriter, r *http.Request) {
	h.renderNewSite(w, r, "", nil)
}

// NewSiteSubmit creates the site and publishes it immediately — there is no
// draft/review step, so the customer's site is live the moment they submit.
func (h *Handler) NewSiteSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	businessName := strings.TrimSpace(r.FormValue("business_name"))
	templateID := r.FormValue("template_id")
	if businessName == "" {
		h.renderNewSite(w, r, "Business name is required.", r.Form)
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
			h.renderNewSite(w, r, verr.Message, r.Form)
			return
		}
		if errors.Is(err, service.ErrSiteLimitReached) {
			h.renderNewSite(w, r, service.ErrSiteLimitReached.Error(), r.Form)
			return
		}
		h.renderNewSite(w, r, "Something went wrong creating your site. Please try again.", r.Form)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d?launched=1", site.ID), http.StatusSeeOther)
}

// GenerateCopy drafts a tagline/about/CTA from a business name + type for
// the "Generate for me" button in the builder wizard. The owner reviews and
// can edit the draft before it's ever saved — this endpoint never writes to
// the database.
func (h *Handler) GenerateCopy(w http.ResponseWriter, r *http.Request) {
	if !h.ai.Configured() {
		http.Error(w, "not available", http.StatusServiceUnavailable)
		return
	}
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if !h.aiGenerateLimiter.Allow(middleware.UserID(r).String()) {
		http.Error(w, "too many requests, try again shortly", http.StatusTooManyRequests)
		return
	}

	businessName := strings.TrimSpace(r.FormValue("business_name"))
	if businessName == "" {
		http.Error(w, "business name is required", http.StatusBadRequest)
		return
	}
	businessType := "general business or trade"
	for _, bt := range businessTypes {
		if bt.ID == r.FormValue("business_type") {
			businessType = bt.Label
			break
		}
	}

	copy, err := h.ai.GenerateSiteCopy(r.Context(), businessName, businessType)
	if err != nil {
		http.Error(w, "couldn't generate content, please try again", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(copy)
}
