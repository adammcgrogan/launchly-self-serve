package web

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

func (h *Handler) AddressForm(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	h.render.Render(w, "dashboard:address", map[string]any{
		"Site":      site,
		"SiteURL":   h.siteURL(site.Slug),
		"Domain":    h.cfg.Domain,
		"CSRFToken": h.csrf.Token(middleware.UserID(r).String()),
		"Flash":     middleware.GetFlash(w, r),
	})
}

func (h *Handler) AddressSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))

	if err := h.sites.RenameSlug(r.Context(), site.ID, slug); err != nil {
		h.render.Render(w, "dashboard:address", map[string]any{
			"Site":      site,
			"SiteURL":   h.siteURL(site.Slug),
			"Domain":    h.cfg.Domain,
			"CSRFToken": h.csrf.Token(middleware.UserID(r).String()),
			"Error":     err.Error(),
			"Slug":      slug,
		})
		return
	}

	middleware.SetFlash(w, "Address updated. Your old link will keep working.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d/address", site.ID), http.StatusSeeOther)
}

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
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
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
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	palette := r.FormValue("palette")
	headingFont := r.FormValue("heading_font")

	tmpl, _ := findTemplate(site.TemplateID)
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
	if headingFont != "" && headingFont != "sans" && headingFont != "serif" {
		headingFont = ""
	}

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
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
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
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := h.sites.Publish(r.Context(), site.ID); err != nil {
		if err == service.ErrSitePaused {
			middleware.SetFlash(w, err.Error())
			http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site published.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) UnpublishSite(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := h.sites.Unpublish(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site unpublished — it's no longer visible to the public.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) DeleteSite(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := h.sites.Delete(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site deleted.")
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (h *Handler) UpdateAnnouncement(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.FormValue("announcement_text"))
	var expiresAt *time.Time
	if raw := strings.TrimSpace(r.FormValue("announcement_expires_at")); raw != "" {
		d, err := time.Parse("2006-01-02", raw)
		if err != nil {
			http.Error(w, "invalid date", http.StatusBadRequest)
			return
		}
		expiresAt = &d
	}

	if err := h.sites.UpdateAnnouncement(r.Context(), site.ID, text, expiresAt); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	if text == "" {
		middleware.SetFlash(w, "Announcement cleared.")
	} else {
		middleware.SetFlash(w, "Announcement saved.")
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) UpdateAnalyticsFrequency(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
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

func (h *Handler) UpdateNotifySettings(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	mobile := strings.TrimSpace(r.FormValue("mobile_number"))
	enabled := r.FormValue("sms_alerts_enabled") == "on"

	if err := h.sites.UpdateNotifySettings(r.Context(), site.ID, mobile, enabled); err != nil {
		if err == service.ErrNotifyNotPro || err == service.ErrNotifyInvalidNumber {
			middleware.SetFlash(w, err.Error())
			http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Notification settings saved.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) SendAnalyticsNow(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := h.cron.SendAnalyticsReport(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Analytics report sent.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}
