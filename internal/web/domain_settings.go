package web

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// DomainSubmit connects (or reconnects) a custom domain to a Pro site.
func (h *Handler) DomainSubmit(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rawDomain := strings.TrimSpace(r.FormValue("domain"))

	ctx, cancel := detachedContext(r)
	defer cancel()
	if _, err := h.domains.SetCustomDomain(ctx, site.ID, rawDomain); err != nil {
		middleware.SetFlash(w, err.Error())
		redirectToSite(w, r, site.ID)
		return
	}

	middleware.SetFlash(w, "Domain added — follow the instructions below to finish connecting it.")
	redirectToSite(w, r, site.ID)
}

// DomainCheckStatus re-checks a pending domain against Cloudflare on demand.
func (h *Handler) DomainCheckStatus(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	ctx, cancel := detachedContext(r)
	defer cancel()
	switch hostname, err := h.domains.RefreshCustomDomainStatus(ctx, site.ID); {
	case err != nil:
		middleware.SetFlash(w, "Couldn't check domain status — try again shortly.")
	case hostname.Active():
		middleware.SetFlash(w, "Your custom domain is live!")
	case hostname.Failed():
		middleware.SetFlash(w, "Domain verification failed — double-check your DNS records.")
	default:
		middleware.SetFlash(w, "Still waiting on DNS — this can take anywhere from a few minutes to a few hours.")
	}
	redirectToSite(w, r, site.ID)
}

// DomainRemove detaches a site's custom domain entirely.
func (h *Handler) DomainRemove(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	ctx, cancel := detachedContext(r)
	defer cancel()
	if err := h.domains.RemoveCustomDomain(ctx, site.ID); err != nil {
		slog.Error("remove custom domain", "site_id", site.ID, "error", err)
		middleware.SetFlash(w, "Couldn't remove the domain — try again.")
	} else {
		middleware.SetFlash(w, "Custom domain removed.")
	}
	redirectToSite(w, r, site.ID)
}
