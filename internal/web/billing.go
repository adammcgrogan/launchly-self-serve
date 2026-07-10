package web

import (
	"fmt"
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
	site := middleware.SiteFromContext(r)
	if r.FormValue("csrf_token") != h.csrf.Token(middleware.UserID(r).String()) {
		http.Error(w, "invalid csrf token", http.StatusForbidden)
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

	checkoutURL, err := h.billing.CreateUpgradeCheckout(r.Context(), site.ID, plan, site.Contact.Email)
	if err != nil {
		slog.Error("create upgrade checkout", "site_id", site.ID, "error", err)
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, checkoutURL, http.StatusSeeOther)
}

func (h *Handler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if err := h.billing.CancelSubscription(r.Context(), site.ID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Subscription cancelled.")
	http.Redirect(w, r, fmt.Sprintf("/dashboard/sites/%d", site.ID), http.StatusSeeOther)
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
