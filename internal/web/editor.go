package web

import (
	"context"
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

// detachedContext returns a context to use for a save that must complete
// even if the client disconnects mid-request. Saves are submitted via
// fetch() (see the [data-ajax-form] JS in site.html), so a user who reloads
// the page while impatiently waiting for a slow save aborts that fetch's
// connection — without this, the write's context would be cancelled too,
// silently rolling back a save the server had already accepted.
func detachedContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Second)
}

// redirectToSite redirects back to a site's dashboard page after a save,
// preserving the tab/subtab/csubtab query params from the page the form was
// submitted on (read from the Referer) so the owner lands back where they
// were instead of the default Overview tab.
func redirectToSite(w http.ResponseWriter, r *http.Request, siteID int) {
	target := fmt.Sprintf("/dashboard/sites/%d", siteID)
	if referer := r.Header.Get("Referer"); referer != "" {
		if u, err := url.Parse(referer); err == nil {
			params := url.Values{}
			for _, key := range []string{"tab", "subtab", "csubtab"} {
				if v := u.Query().Get(key); v != "" {
					params.Set(key, v)
				}
			}
			if len(params) > 0 {
				target += "?" + params.Encode()
			}
		}
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (h *Handler) AddressSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))

	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.sites.RenameSlug(ctx, site.ID, slug); err != nil {
		middleware.SetFlash(w, err.Error())
		redirectToSite(w, r, site.ID)
		return
	}

	middleware.SetFlash(w, "Address updated. Your old link will keep working.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) EditSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	in := service.UpdateContentInput{
		SiteID:          site.ID,
		BusinessName:    strings.TrimSpace(r.FormValue("business_name")),
		Tagline:         strings.TrimSpace(r.FormValue("tagline")),
		About:           strings.TrimSpace(r.FormValue("about")),
		LogoURL:         strings.TrimSpace(r.FormValue("logo_url")),
		CTAText:         strings.TrimSpace(r.FormValue("cta_text")),
		VideoURL:        strings.TrimSpace(r.FormValue("video_url")),
		Timezone:        resolveTimezone(r.FormValue("timezone")),
		MetaTitle:       strings.TrimSpace(r.FormValue("meta_title")),
		MetaDescription: strings.TrimSpace(r.FormValue("meta_description")),
		OgImageURL:      strings.TrimSpace(r.FormValue("og_image_url")),
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
		Certifications: parseCertificationRows(r),
		Testimonials:   parseTestimonialRows(r),
		GalleryImages:  parseGalleryRows(r),
		FAQItems:       parseFAQRows(r),
		StaffMembers:   parseStaffRows(r),
		BusinessHours:  parseBusinessHours(r),
		SpecialHours:   parseSpecialHoursRows(r),
		ServiceAreas:   parseServiceAreaRows(r),
		Reviews: domain.SiteReviews{
			Rating:      strings.TrimSpace(r.FormValue("review_rating")),
			ReviewCount: atoiClamp(r.FormValue("review_count")),
			ReviewURL:   strings.TrimSpace(r.FormValue("review_url")),
		},
	}

	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.sites.UpdateContent(ctx, in); err != nil {
		var verr *service.ValidationError
		if errors.As(err, &verr) {
			middleware.SetFlash(w, verr.Message)
			redirectToSite(w, r, site.ID)
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Changes saved.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) AppearanceSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
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
		redirectToSite(w, r, site.ID)
		return
	}

	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.sites.UpdateAppearance(ctx, site.ID, palette, headingFont, brandColor); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Appearance saved.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) SwitchTemplateSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
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
	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.sites.SwitchTemplate(ctx, site.ID, templateID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Template switched.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) FormTypeSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
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
	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.sites.UpdateFormType(ctx, site.ID, formType); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Form type saved.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) LeadStatusSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
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
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.sites.Publish(ctx, site.ID); err != nil {
		if err == service.ErrSitePaused {
			middleware.SetFlash(w, err.Error())
			redirectToSite(w, r, site.ID)
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site published.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) UnpublishSite(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.sites.Unpublish(ctx, site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site unpublished — it's no longer visible to the public.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) DeleteSite(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
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
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
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

	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.sites.UpdateAnnouncement(ctx, site.ID, text, expiresAt, tone, linkURL, linkLabel); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	if text == "" {
		middleware.SetFlash(w, "Announcement cleared.")
	} else {
		middleware.SetFlash(w, "Announcement saved.")
	}
	redirectToSite(w, r, site.ID)
}

func (h *Handler) UpdateAnalyticsFrequency(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	freq := r.FormValue("analytics_frequency")
	if freq != "off" && freq != "monthly" {
		http.Error(w, "invalid frequency", http.StatusBadRequest)
		return
	}
	if err := h.sites.UpdateAnalyticsFrequency(r.Context(), site.ID, freq); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	redirectToSite(w, r, site.ID)
}

func (h *Handler) UpdateNotifySettings(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
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
		redirectToSite(w, r, site.ID)
		return
	}

	if err := h.sites.UpdateNotifySettings(r.Context(), site.ID, mobile, enabled); err != nil {
		if err == service.ErrNotifyNotPro || err == service.ErrNotifyInvalidNumber {
			middleware.SetFlash(w, err.Error())
			redirectToSite(w, r, site.ID)
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Notification settings saved.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) UpdateTrackingSettings(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.sites.UpdateTrackingSettings(r.Context(), site.ID,
		r.FormValue("ga_measurement_id"), r.FormValue("meta_pixel_id")); err != nil {
		if err == service.ErrTrackingNotPro || err == service.ErrTrackingInvalid {
			middleware.SetFlash(w, err.Error())
			redirectToSite(w, r, site.ID)
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Tracking settings saved.")
	redirectToSite(w, r, site.ID)
}

func (h *Handler) SendAnalyticsNow(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := h.cron.SendAnalyticsReport(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Analytics report sent.")
	redirectToSite(w, r, site.ID)
}
