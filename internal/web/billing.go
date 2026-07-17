package web

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// UpgradeCheckout starts a self-serve Stripe Checkout session for a plan
// upgrade — the customer initiates this themselves from their dashboard,
// there is no admin-sent payment link.
func (h *Handler) UpgradeCheckout(w http.ResponseWriter, r *http.Request) {
	site := middleware.LightSiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	plan := domain.Plan(r.FormValue("plan"))
	if plan != domain.PlanStarter && plan != domain.PlanPro {
		http.Error(w, "invalid plan", http.StatusBadRequest)
		return
	}

	// Bill the account owner's login email, not the site's public contact
	// email — the two can differ (or the public one can be left blank).
	var customerEmail string
	if contact, err := h.sites.GetSiteContact(r.Context(), site.ID); err == nil && contact != nil {
		customerEmail = contact.Email
	}
	if profile, err := h.accounts.GetProfile(r.Context(), middleware.UserID(r)); err == nil && profile != nil && profile.Email != "" {
		customerEmail = profile.Email
	}

	checkoutURL, err := h.billing.CreateUpgradeCheckout(r.Context(), site.ID, site.Slug, plan, customerEmail)
	if err != nil {
		slog.Error("create upgrade checkout", "site_id", site.ID, "error", err)
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
}

func (h *Handler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	site := middleware.LightSiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := h.billing.CancelSubscription(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Subscription cancelled.")
	http.Redirect(w, r, "/dashboard/sites/"+site.Slug, http.StatusSeeOther)
}

// StripeWebhook is the single source of truth for payment state — Stripe
// calls this directly, no admin action is involved anywhere in the path.
func (h *Handler) StripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	event, err := h.billing.ParseWebhook(body, r.Header.Get("Stripe-Signature"))
	if err != nil {
		slog.Error("stripe webhook parse", "error", err)
		http.Error(w, "invalid webhook", http.StatusBadRequest)
		return
	}
	if err := h.billing.HandleWebhookEvent(r.Context(), event); err != nil {
		slog.Error("handle stripe webhook", "error", err)
		http.Error(w, "processing error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}
