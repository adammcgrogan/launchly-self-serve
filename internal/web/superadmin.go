package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// Superadmin is a cross-account view for Adam — nothing here is on the path
// a customer's site or payment depends on. It's gated by a single shared
// password (an env var), entirely separate from customer Supabase accounts.

func (h *Handler) SuperadminLoginForm(w http.ResponseWriter, r *http.Request) {
	if h.superadmin.IsAuthenticated(r) {
		http.Redirect(w, r, "/superadmin", http.StatusSeeOther)
		return
	}
	h.render.Render(w, "superadmin:login", map[string]any{"Next": r.URL.Query().Get("next")})
}

func (h *Handler) SuperadminLoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.loginLimiter.Allow(middleware.ClientIP(r)) {
		h.render.Render(w, "superadmin:login", map[string]any{"Error": "Too many attempts. Please wait a moment and try again."})
		return
	}
	if !h.superadmin.CheckPassword(r.FormValue("password")) {
		h.render.Render(w, "superadmin:login", map[string]any{"Error": "Incorrect password.", "Next": r.FormValue("next")})
		return
	}
	h.superadmin.SetSession(w)
	next := r.FormValue("next")
	if next == "" {
		next = "/superadmin"
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *Handler) SuperadminLogout(w http.ResponseWriter, r *http.Request) {
	h.superadmin.ClearSession(w)
	http.Redirect(w, r, "/superadmin/login", http.StatusSeeOther)
}

func (h *Handler) SuperadminDashboard(w http.ResponseWriter, r *http.Request) {
	sites, err := h.sites.ListAllSites(r.Context())
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	stats, err := h.sites.PlatformStats(r.Context())
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.render.Render(w, "superadmin:dashboard", map[string]any{
		"Sites": sites,
		"Stats": stats,
		"Flash": middleware.GetFlash(w, r),
	})
}

func (h *Handler) SuperadminSiteView(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	site, err := h.sites.GetSiteAggregate(r.Context(), id)
	if err != nil || site == nil {
		http.NotFound(w, r)
		return
	}
	leads, _ := h.leads.ListBySite(r.Context(), id)
	h.render.Render(w, "superadmin:site", map[string]any{
		"Site":             site,
		"Leads":            leads,
		"SiteURL":          h.siteURL(site.Slug),
		"CSRFToken":        h.csrf.Token("superadmin", ""),
		"Flash":            middleware.GetFlash(w, r),
		"Socials":          socialLinksMap(site.SocialLinks),
		"ServiceRows":      serviceRowsForDisplay(site.Services),
		"CertRows":         certificationRowsForDisplay(site.Certifications),
		"AreaRows":         serviceAreaRowsForDisplay(site.ServiceAreas),
		"Reviews":          site.Reviews,
		"TestimonialRows":  testimonialRowsForDisplay(site.Testimonials),
		"GalleryRows":      galleryRowsForDisplay(site.GalleryImages),
		"FAQRows":          faqRowsForDisplay(site.FAQItems),
		"StaffRows":        staffRowsForDisplay(site.StaffMembers),
		"HoursByDay":       businessHoursByDay(site.BusinessHours),
		"SpecialHoursRows": specialHoursRowsForDisplay(site.SpecialHours),
		"Weekdays":         weekdays,
		"Timezones":        timezones,
	})
}

// SuperadminEditSubmit lets an admin edit any site's content — the same
// fields and service.UpdateContent call the owner-facing editor uses (see
// EditSubmit) — just reached via the superadmin route/session instead of
// site ownership.
func (h *Handler) SuperadminEditSubmit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !h.checkCSRF(w, r, "superadmin", "") {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	in := buildUpdateContentInput(r, id)
	if err := h.sites.UpdateContent(r.Context(), in); err != nil {
		var verr *service.ValidationError
		if errors.As(err, &verr) {
			middleware.SetFlash(w, verr.Message)
			http.Redirect(w, r, "/superadmin/sites/"+strconv.Itoa(id), http.StatusSeeOther)
			return
		}
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	middleware.SetFlash(w, "Changes saved.")
	http.Redirect(w, r, "/superadmin/sites/"+strconv.Itoa(id), http.StatusSeeOther)
}

// SuperadminUnpublish and SuperadminDelete are the emergency abuse-handling
// backstop — unlike SuperadminEditSubmit, they're not part of the normal
// support flow.
func (h *Handler) SuperadminUnpublish(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !h.checkCSRF(w, r, "superadmin", "") {
		return
	}
	if err := h.sites.Unpublish(r.Context(), id); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site unpublished.")
	http.Redirect(w, r, "/superadmin/sites/"+strconv.Itoa(id), http.StatusSeeOther)
}

func (h *Handler) SuperadminDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if !h.checkCSRF(w, r, "superadmin", "") {
		return
	}
	if err := h.sites.Delete(r.Context(), id); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site deleted.")
	http.Redirect(w, r, "/superadmin", http.StatusSeeOther)
}
