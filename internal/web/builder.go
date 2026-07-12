package web

import (
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
		"Templates":     siteTemplates,
		"Error":         errMsg,
		"Values":        values,
		"CSRFToken":     h.csrf.Token(middleware.UserID(r).String()),
		"EmailVerified": h.emailVerified(r),
	})
}

// NewSiteForm renders the builder wizard: pick a template, fill in content.
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
	if _, ok := findTemplate(templateID); !ok {
		templateID = siteTemplates[0].ID
	}

	in := service.CreateSiteInput{
		OwnerUserID:  middleware.UserID(r),
		BusinessName: businessName,
		Tagline:      strings.TrimSpace(r.FormValue("tagline")),
		About:        strings.TrimSpace(r.FormValue("about")),
		LogoURL:      strings.TrimSpace(r.FormValue("logo_url")),
		CTAText:      strings.TrimSpace(r.FormValue("cta_text")),
		TemplateID:   templateID,
		Contact: domain.SiteContact{
			Phone:       strings.TrimSpace(r.FormValue("phone")),
			Email:       strings.TrimSpace(r.FormValue("email")),
			Address:     strings.TrimSpace(r.FormValue("address")),
			Location:    strings.TrimSpace(r.FormValue("location")),
			MapURL:      strings.TrimSpace(r.FormValue("map_url")),
			MapEmbedURL: strings.TrimSpace(r.FormValue("map_embed_url")),
		},
		SocialLinks:    parseSocialLinks(r),
		Services:       parseServices(r.FormValue("services")),
		Certifications: parseCertifications(r.FormValue("certifications")),
		Testimonials:   parseTestimonials(r.FormValue("testimonials")),
		GalleryImages:  parseGallery(r.FormValue("gallery")),
		BusinessHours:  parseBusinessHours(r.FormValue("hours")),
	}

	site, err := h.sites.CreateSite(r.Context(), in)
	if err != nil {
		h.renderNewSite(w, r, "Something went wrong creating your site. Please try again.", r.Form)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d?launched=1", site.ID), http.StatusSeeOther)
}
