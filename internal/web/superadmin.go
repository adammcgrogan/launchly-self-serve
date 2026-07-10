package web

import (
	"net/http"
	"strconv"

	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// Superadmin is a read-mostly cross-account view for Adam — nothing here is
// on the path a customer's site or payment depends on. It's gated by a
// single shared password (an env var), entirely separate from customer
// Supabase accounts.

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
	h.render.Render(w, "superadmin:dashboard", map[string]any{
		"Sites": sites,
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
		"Site":      site,
		"Leads":     leads,
		"SiteURL":   h.siteURL(site.Slug),
		"CSRFToken": h.csrf.Token("superadmin"),
		"Flash":     middleware.GetFlash(w, r),
	})
}

// SuperadminUnpublish and SuperadminDelete are the emergency abuse-handling
// backstop — the only two actions in the whole app superadmin can take that
// affect a customer's site.
func (h *Handler) SuperadminUnpublish(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if r.FormValue("csrf_token") != h.csrf.Token("superadmin") {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
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
	if r.FormValue("csrf_token") != h.csrf.Token("superadmin") {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
		return
	}
	if err := h.sites.Delete(r.Context(), id); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Site deleted.")
	http.Redirect(w, r, "/superadmin", http.StatusSeeOther)
}
