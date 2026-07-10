package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

func (h *Handler) EditForm(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	socials := socialLinksMap(site.SocialLinks)
	h.render.Render(w, "dashboard:edit", map[string]any{
		"Site":             site,
		"Socials":          socials,
		"ServicesText":     servicesToLines(site.Services),
		"CertsText":        certificationsToLines(site.Certifications),
		"TestimonialsText": testimonialsToLines(site.Testimonials),
		"GalleryText":      galleryToLines(site.GalleryImages),
		"HoursText":        businessHoursToLines(site.BusinessHours),
		"CSRFToken":        h.csrf.Token(middleware.UserID(r).String()),
	})
}

func (h *Handler) EditSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if r.FormValue("csrf_token") != h.csrf.Token(middleware.UserID(r).String()) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	in := service.UpdateContentInput{
		SiteID:       site.ID,
		BusinessName: strings.TrimSpace(r.FormValue("business_name")),
		Tagline:      strings.TrimSpace(r.FormValue("tagline")),
		About:        strings.TrimSpace(r.FormValue("about")),
		LogoURL:      strings.TrimSpace(r.FormValue("logo_url")),
		CTAText:      strings.TrimSpace(r.FormValue("cta_text")),
		Contact: domain.SiteContact{
			SiteID:      site.ID,
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

	if err := h.sites.UpdateContent(r.Context(), in); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Changes saved.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) AppearanceForm(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	tmpl, _ := findTemplate(site.TemplateID)
	h.render.Render(w, "dashboard:appearance", map[string]any{
		"Site":      site,
		"Palettes":  tmpl.Palettes,
		"CSRFToken": h.csrf.Token(middleware.UserID(r).String()),
	})
}

func (h *Handler) AppearanceSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if r.FormValue("csrf_token") != h.csrf.Token(middleware.UserID(r).String()) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	palette := r.FormValue("palette")
	headingFont := r.FormValue("heading_font")
	if err := h.sites.UpdateAppearance(r.Context(), site.ID, palette, headingFont); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Appearance saved.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) SwitchTemplateForm(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	h.render.Render(w, "dashboard:switch_template", map[string]any{
		"Site":      site,
		"Templates": siteTemplates,
		"CSRFToken": h.csrf.Token(middleware.UserID(r).String()),
	})
}

func (h *Handler) SwitchTemplateSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if r.FormValue("csrf_token") != h.csrf.Token(middleware.UserID(r).String()) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	templateID := r.FormValue("template_id")
	if _, ok := findTemplate(templateID); !ok {
		http.Error(w, "invalid template", http.StatusBadRequest)
		return
	}
	if err := h.sites.SwitchTemplate(r.Context(), site.ID, templateID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Template switched.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) PublishSite(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if err := h.sites.Publish(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site published.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) UnpublishSite(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if err := h.sites.Unpublish(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site unpublished — it's no longer visible to the public.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) DeleteSite(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if err := h.sites.Delete(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site deleted.")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handler) UpdateAnalyticsFrequency(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	freq := r.FormValue("analytics_frequency")
	if freq != "off" && freq != "weekly" && freq != "monthly" {
		http.Error(w, "invalid frequency", http.StatusBadRequest)
		return
	}
	if err := h.sites.UpdateAnalyticsFrequency(r.Context(), site.ID, freq); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) SendAnalyticsNow(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if err := h.cron.SendAnalyticsReport(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Analytics report sent.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}
