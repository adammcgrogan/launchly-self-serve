package web

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// Dashboard lists every site the logged-in user owns.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	sites, err := h.sites.ListSitesByOwner(r.Context(), middleware.UserID(r))
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.render.Render(w, "dashboard:sites", map[string]any{
		"Sites": sites,
		"Flash": middleware.GetFlash(w, r),
	})
}

// SiteOverview shows one site's status, live URL, trial/billing state,
// stats, and recent leads. RequireSiteOwner has already loaded the site
// into the request context.
func (h *Handler) SiteOverview(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)

	if r.URL.Query().Get("launched") == "1" {
		siteURL := h.siteURL(site.Slug)
		h.render.Render(w, "dashboard:launched", map[string]any{
			"Site":    site,
			"SiteURL": siteURL,
		})
		return
	}

	leads, err := h.leads.ListBySite(r.Context(), site.ID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	since7 := time.Now().UTC().Add(-7 * 24 * time.Hour)
	stats, _ := h.analytics.GetSiteStats(r.Context(), site.ID, since7)

	h.render.Render(w, "dashboard:site", map[string]any{
		"Site":               site,
		"Leads":              leads,
		"Stats":              stats,
		"SiteURL":            h.siteURL(site.Slug),
		"Flash":              middleware.GetFlash(w, r),
		"CSRFToken":          h.csrf.Token(middleware.UserID(r).String()),
		"Upgraded":           r.URL.Query().Get("upgraded") == "1",
		"SMSAlertsAvailable": h.cfg.SMSAlertsAvailable(),
	})
}

// Account shows the logged-in user's email and account-level actions
// (password reset goes through Supabase's own recovery email flow).
func (h *Handler) Account(w http.ResponseWriter, r *http.Request) {
	profile, err := h.accounts.GetProfile(r.Context(), middleware.UserID(r))
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.render.Render(w, "dashboard:account", map[string]any{
		"Profile": profile,
		"Flash":   middleware.GetFlash(w, r),
	})
}

func (h *Handler) ExportLeads(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	leads, err := h.leads.ListBySite(r.Context(), site.ID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-leads.csv"`, site.Slug))
	cw := csv.NewWriter(w)
	cw.Write([]string{"Name", "Email", "Phone", "Service", "Preferred time", "Message", "Date"})
	for _, l := range leads {
		cw.Write([]string{l.Name, l.Email, l.Phone, l.ServiceLabel, l.PreferredTime, l.Message, l.CreatedAt.Format("2006-01-02 15:04")})
	}
	cw.Flush()
}
