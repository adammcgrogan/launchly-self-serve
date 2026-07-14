package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

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
		middleware.SetFlash(w, err.Error())
		http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
		return
	}

	middleware.SetFlash(w, "Address updated. Your old link will keep working.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
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
		Timezone:     resolveTimezone(r.FormValue("timezone")),
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
		Services:       parseServiceRows(r),
		Certifications: parseCertifications(r.FormValue("certifications")),
		Testimonials:   parseTestimonials(r.FormValue("testimonials")),
		GalleryImages:  parseGallery(r.FormValue("gallery")),
		FAQItems:       parseFAQRows(r),
		StaffMembers:   parseStaffRows(r),
		BusinessHours:  parseBusinessHours(r),
		ServiceAreas:   parseServiceAreas(r.FormValue("service_areas")),
		Reviews: domain.SiteReviews{
			Rating:      strings.TrimSpace(r.FormValue("review_rating")),
			ReviewCount: atoiClamp(r.FormValue("review_count")),
			ReviewURL:   strings.TrimSpace(r.FormValue("review_url")),
		},
	}

	if err := h.sites.UpdateContent(r.Context(), in); err != nil {
		var verr *service.ValidationError
		if errors.As(err, &verr) {
			middleware.SetFlash(w, verr.Message)
			http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Changes saved.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
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
	brandColor := strings.ToUpper(strings.TrimSpace(r.FormValue("brand_color")))
	if brandColor != "" && !strings.HasPrefix(brandColor, "#") {
		brandColor = "#" + brandColor
	}

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
	if brandColor != "" && !service.IsValidHexColor(brandColor) {
		middleware.SetFlash(w, "Brand colour must be a 6-digit hex code, e.g. #4F46E5.")
		http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
		return
	}

	if err := h.sites.UpdateAppearance(r.Context(), site.ID, palette, headingFont, brandColor); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Appearance saved.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
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

func (h *Handler) FormTypeSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	formType := domain.FormType(r.FormValue("form_type"))
	if formType != domain.FormTypeContact && formType != domain.FormTypeBooking {
		http.Error(w, "invalid form type", http.StatusBadRequest)
		return
	}
	if err := h.sites.UpdateFormType(r.Context(), site.ID, formType); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Form type saved.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
}

func (h *Handler) LeadStatusSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	leadID, err := strconv.Atoi(r.PathValue("leadID"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	status := domain.LeadStatus(r.FormValue("status"))
	switch status {
	case domain.LeadStatusNew, domain.LeadStatusContacted, domain.LeadStatusWon, domain.LeadStatusLost:
	default:
		http.Error(w, "invalid status", http.StatusBadRequest)
		return
	}
	if err := h.leads.UpdateStatus(r.Context(), site.ID, leadID, status); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	// Called via fetch() from the lead-status pill, not a form submit, so
	// there's no page to redirect back to.
	w.WriteHeader(http.StatusNoContent)
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
	tone := domain.AnnouncementTone(r.FormValue("announcement_tone"))
	switch tone {
	case domain.AnnouncementInfo, domain.AnnouncementPromo, domain.AnnouncementUrgent:
	default:
		tone = domain.AnnouncementInfo
	}
	linkURL := strings.TrimSpace(r.FormValue("announcement_link_url"))
	linkLabel := strings.TrimSpace(r.FormValue("announcement_link_label"))
	if linkURL != "" {
		u, err := url.Parse(linkURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			http.Error(w, "invalid link URL", http.StatusBadRequest)
			return
		}
	}
	if linkURL == "" {
		linkLabel = ""
	}

	if err := h.sites.UpdateAnnouncement(r.Context(), site.ID, text, expiresAt, tone, linkURL, linkLabel); err != nil {
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
	if enabled && !h.cfg.SMSAlertsAvailable() {
		middleware.SetFlash(w, "SMS lead alerts aren't available yet.")
		http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
		return
	}

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
