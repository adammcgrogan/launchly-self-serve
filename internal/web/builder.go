package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// NewSiteForm renders the builder wizard: pick a template, fill in content.
func (h *Handler) NewSiteForm(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "dashboard:new_site", map[string]any{
		"Templates": siteTemplates,
		"CSRFToken": h.csrf.Token(middleware.UserID(r).String()),
	})
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
		h.render.Render(w, "dashboard:new_site", map[string]any{
			"Templates": siteTemplates, "Error": "Business name is required.",
			"CSRFToken": h.csrf.Token(middleware.UserID(r).String()),
		})
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
		h.render.Render(w, "dashboard:new_site", map[string]any{
			"Templates": siteTemplates, "Error": "Something went wrong creating your site — please try again.",
			"CSRFToken": h.csrf.Token(middleware.UserID(r).String()),
		})
		return
	}

	middleware.SetFlash(w, "Your site is live!")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}
