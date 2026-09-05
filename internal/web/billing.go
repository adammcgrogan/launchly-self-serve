package web

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// UpgradeCheckout starts a self-serve Stripe Checkout session for a plan
// upgrade — the customer initiates this themselves from their dashboard,
// there is no admin-sent payment link. The plan is the account's (#338), so
// this covers every site they own; the site in hand only decides where they
// come back to.
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

	checkoutURL, err := h.billing.CreateUpgradeCheckout(r.Context(), site.OwnerUserID, site.Slug, plan, customerEmail)
	if err != nil {
		slog.Error("create upgrade checkout", "owner_user_id", site.OwnerUserID, "error", err)
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
}

// BillingPortal redirects the owner into Stripe's billing portal, where they
// can replace a failing card, see invoices and manage the subscription (see
// #316). The portal URL is single-use and short-lived, so it's minted per
// click here rather than rendered into the page or an email.
func (h *Handler) BillingPortal(w http.ResponseWriter, r *http.Request) {
	site := middleware.LightSiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	portalURL, err := h.billing.CreateBillingPortalSession(r.Context(), site.OwnerUserID, site.Slug)
	if err != nil {
		// A site that never completed a checkout has no Stripe customer and
		// so nothing to manage — that's a wrong turn, not a server error.
		if errors.Is(err, service.ErrNoBillingCustomer) {
			middleware.SetFlash(w, "There's no billing to manage yet — upgrade first.")
			http.Redirect(w, r, "/dashboard/sites/"+site.Slug+"/billing", http.StatusSeeOther)
			return
		}
		slog.Error("create billing portal session", "owner_user_id", site.OwnerUserID, "error", err)
		middleware.SetFlash(w, "We couldn't open the billing portal. Please try again, or email hello@launchly.ltd.")
		http.Redirect(w, r, "/dashboard/sites/"+site.Slug+"/billing", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, portalURL, http.StatusSeeOther)
}

// CancelSubscription cancels the account's whole subscription, pausing every
// site it owns — one plan covers them all (#338), so there is no per-site
// cancel. The confirmation copy on the billing page says so explicitly.
func (h *Handler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	site := middleware.LightSiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := h.billing.CancelSubscription(r.Context(), site.OwnerUserID); err != nil {
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
