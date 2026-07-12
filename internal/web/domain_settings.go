package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// DomainForm shows a Pro site's custom domain settings: an upsell for
// Starter sites, otherwise the current domain (if any) and setup
// instructions. Pending domains are re-checked against Cloudflare on every
// load so the required DNS records stay visible and the status catches up
// automatically once DNS propagates.
func (h *Handler) DomainForm(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	data := map[string]any{
		"Site":           site,
		"CSRFToken":      h.csrf.Token(middleware.UserID(r).String()),
		"Flash":          middleware.GetFlash(w, r),
		"FallbackOrigin": h.domains.FallbackOrigin(),
		"IsPro":          site.Billing.Plan == domain.PlanPro,
	}
	if site.CustomDomain != "" && site.CustomDomainStatus == domain.CustomDomainPending {
		if hostname, err := h.domains.RefreshCustomDomainStatus(r.Context(), site.ID); err == nil {
			data["Hostname"] = hostname
			if hostname.Active() {
				site.CustomDomainStatus = domain.CustomDomainActive
			} else if hostname.Failed() {
				site.CustomDomainStatus = domain.CustomDomainFailed
			}
		}
	}
	h.render.Render(w, "dashboard:domain", data)
}

// DomainSubmit connects (or reconnects) a custom domain to a Pro site.
func (h *Handler) DomainSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rawDomain := strings.TrimSpace(r.FormValue("domain"))

	if _, err := h.domains.SetCustomDomain(r.Context(), site.ID, rawDomain); err != nil {
		h.render.Render(w, "dashboard:domain", map[string]any{
			"Site":           site,
			"CSRFToken":      h.csrf.Token(middleware.UserID(r).String()),
			"FallbackOrigin": h.domains.FallbackOrigin(),
			"IsPro":          site.Billing.Plan == domain.PlanPro,
			"Error":          err.Error(),
			"DomainInput":    rawDomain,
		})
		return
	}

	middleware.SetFlash(w, "Domain added — follow the instructions below to finish connecting it.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d/domain", site.ID), http.StatusSeeOther)
}

// DomainCheckStatus re-checks a pending domain against Cloudflare on demand.
func (h *Handler) DomainCheckStatus(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	switch hostname, err := h.domains.RefreshCustomDomainStatus(r.Context(), site.ID); {
	case err != nil:
		middleware.SetFlash(w, "Couldn't check domain status — try again shortly.")
	case hostname.Active():
		middleware.SetFlash(w, "Your custom domain is live!")
	case hostname.Failed():
		middleware.SetFlash(w, "Domain verification failed — double-check your DNS records.")
	default:
		middleware.SetFlash(w, "Still waiting on DNS — this can take anywhere from a few minutes to a few hours.")
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d/domain", site.ID), http.StatusSeeOther)
}

// DomainRemove detaches a site's custom domain entirely.
func (h *Handler) DomainRemove(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String()) {
		return
	}
	if err := h.domains.RemoveCustomDomain(r.Context(), site.ID); err != nil {
		slog.Error("remove custom domain", "site_id", site.ID, "error", err)
		middleware.SetFlash(w, "Couldn't remove the domain — try again.")
	} else {
		middleware.SetFlash(w, "Custom domain removed.")
	}
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d/domain", site.ID), http.StatusSeeOther)
}
