package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// InviteMember sends a teammate invite for the current site. Owner-only —
// wired through middleware.Ownership.RequireOwnerRole in the router.
func (h *Handler) InviteMember(w http.ResponseWriter, r *http.Request) {
	site := middleware.LightSiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	inviterEmail := ""
	if profile, err := h.accounts.GetProfile(r.Context(), middleware.UserID(r)); err == nil && profile != nil {
		inviterEmail = profile.Email
	}

	ctx, cancel := detachedContext(r)
	defer cancel()
	if _, err := h.members.Invite(ctx, site, inviterEmail, r.FormValue("email")); err != nil {
		var verr *service.ValidationError
		if errors.As(err, &verr) {
			middleware.SetFlash(w, verr.Message)
		} else {
			middleware.SetFlash(w, err.Error())
		}
		redirectToSite(w, r, site.Slug)
		return
	}

	middleware.SetFlash(w, "Invite sent.")
	redirectToSite(w, r, site.Slug)
}

// RemoveMember revokes a teammate's access (or withdraws a pending invite).
// Owner-only.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	site := middleware.LightSiteFromContext(r)
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}
	memberID, err := strconv.Atoi(r.PathValue("memberID"))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := h.members.Remove(r.Context(), site.ID, memberID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	middleware.SetFlash(w, "Access removed.")
	redirectToSite(w, r, site.Slug)
}

// AcceptInviteForm shows a confirmation page for a team invite link. The
// route sits under /dashboard so an unauthenticated visitor gets redirected
// to /login?next=/dashboard/invites/{token} and, after signing up or
// logging in, lands right back here — no special-casing of the "next"
// redirect logic in auth.go needed.
func (h *Handler) AcceptInviteForm(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	invite, err := h.members.GetByToken(r.Context(), token)
	if err != nil || invite == nil {
		h.render.Render(w, "dashboard:accept_invite", map[string]any{
			"Error": "That invite link is invalid or has expired.",
		})
		return
	}
	site, err := h.sites.GetSiteByID(r.Context(), invite.SiteID)
	if err != nil || site == nil {
		h.render.Render(w, "dashboard:accept_invite", map[string]any{
			"Error": "That invite link is invalid or has expired.",
		})
		return
	}

	h.render.Render(w, "dashboard:accept_invite", map[string]any{
		"Invite":    invite,
		"Site":      site,
		"CSRFToken": h.csrf.Token(middleware.UserID(r).String(), h.auth.SessionNonce(r)),
	})
}

// AcceptInviteSubmit binds the invite to the logged-in user and drops them
// straight onto the site's dashboard.
func (h *Handler) AcceptInviteSubmit(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if !h.checkCSRF(w, r, middleware.UserID(r).String(), h.auth.SessionNonce(r)) {
		return
	}

	profile, err := h.accounts.GetProfile(r.Context(), middleware.UserID(r))
	if err != nil || profile == nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	member, err := h.members.Accept(r.Context(), token, middleware.UserID(r), profile.Email)
	if err != nil {
		h.render.Render(w, "dashboard:accept_invite", map[string]any{"Error": err.Error()})
		return
	}

	site, err := h.sites.GetSiteByID(r.Context(), member.SiteID)
	if err != nil || site == nil {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}
	middleware.SetFlash(w, "You now have access to "+site.BusinessName+".")
	http.Redirect(w, r, "/dashboard/sites/"+site.Slug, http.StatusSeeOther)
}
