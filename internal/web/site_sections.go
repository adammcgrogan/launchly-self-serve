package web

import (
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// siteSection is one entry in the site dashboard's left-hand nav. Each
// section is a real URL under /dashboard/sites/{slug} rather than a
// client-side tab, so every screen is linkable, bookmarkable, and only
// loads the data it actually shows (see #301).
type siteSection struct {
	Key       string // URL path segment; "" is the overview, at the site root
	Label     string
	Hint      string // one-line description shown under the section heading
	OwnerOnly bool   // hidden from (and unreachable by) team members
}

var siteSections = []siteSection{
	{Key: "", Label: "Overview", Hint: "Your traffic, enquiries, and setup progress."},
	{Key: "content", Label: "Content", Hint: "Everything that appears on your site, in one page."},
	{Key: "design", Label: "Design", Hint: "Colours, fonts, and which design your site uses."},
	{Key: "form", Label: "Form", Hint: "What your site's enquiry form asks visitors for."},
	{Key: "domain", Label: "Domain", Hint: "Your Launchly address and any domain you own."},
	{Key: "publishing", Label: "Publishing", Hint: "Take your site live, share it, or export your data."},
	{Key: "notifications", Label: "Notifications", Hint: "What Launchly emails you, and how often."},
	{Key: "access", Label: "Access", Hint: "Teammates who can see leads and edit this site.", OwnerOnly: true},
	{Key: "billing", Label: "Billing", Hint: "Your plan and subscription.", OwnerOnly: true},
}

// sectionPath returns the dashboard URL for one of a site's sections.
func sectionPath(slug, key string) string {
	if key == "" {
		return "/dashboard/sites/" + slug
	}
	return "/dashboard/sites/" + slug + "/" + key
}

// isSitePath reports whether p is the dashboard path of one of slug's
// sections — used to decide whether a Referer is safe to redirect back to
// after a save.
func isSitePath(slug, p string) bool {
	for _, s := range siteSections {
		if p == sectionPath(slug, s.Key) {
			return true
		}
	}
	return false
}

// siteSectionData assembles the template data every section of the site
// dashboard shares: the site itself, the nav's state, flash/CSRF, and the
// trial/past-due banners rendered by site_layout.html.
func (h *Handler) siteSectionData(w http.ResponseWriter, r *http.Request, site *domain.SiteAggregate, section string) map[string]any {
	var trialDaysLeft int
	showTrialBanner := site.Billing.PaymentStatus == domain.PaymentStatusTrialing && site.Billing.TrialEndsAt != nil
	if showTrialBanner {
		trialDaysLeft = int(math.Ceil(time.Until(*site.Billing.TrialEndsAt).Hours() / 24))
		if trialDaysLeft < 0 {
			trialDaysLeft = 0
		}
	}

	_, checklistPercent := siteChecklist(site)
	var current siteSection
	for _, s := range siteSections {
		if s.Key == section {
			current = s
		}
	}

	return map[string]any{
		"Site":              site,
		"SiteURL":           h.siteURL(site.Slug),
		"Section":           section,
		"SectionLabel":      current.Label,
		"SectionHint":       current.Hint,
		"Sections":          siteSections,
		"ChecklistPercent":  checklistPercent,
		"IsOwner":           site.OwnerUserID == middleware.UserID(r),
		"Flash":             middleware.GetFlash(w, r),
		"CSRFToken":         h.csrf.Token(middleware.UserID(r).String(), h.auth.SessionNonce(r)),
		"EmailVerified":     h.emailVerified(r),
		"Upgraded":          r.URL.Query().Get("upgraded") == "1",
		"ShowTrialBanner":   showTrialBanner,
		"TrialDaysLeft":     trialDaysLeft,
		"ShowPastDueBanner": site.Billing.PaymentStatus == domain.PaymentStatusPastDue,
	}
}

// renderSection renders one section of the site dashboard: the shared
// layout data merged with whatever extra data the section itself needs.
func (h *Handler) renderSection(w http.ResponseWriter, r *http.Request, site *domain.SiteAggregate, section string, extra map[string]any) {
	data := h.siteSectionData(w, r, site, section)
	for k, v := range extra {
		data[k] = v
	}
	h.render.Render(w, "dashboard:site_"+sectionTemplate(section), data)
}

// sectionTemplate maps a section key to its template suffix — the overview
// lives at the site root, so its key is empty but its template isn't.
func sectionTemplate(section string) string {
	if section == "" {
		return "overview"
	}
	return section
}

// requireSiteOwnerRole guards the owner-only sections (Access, Billing) at
// render time. The mutating routes behind them already have their own
// ownerOnly middleware; this just keeps a member from loading the page.
func (h *Handler) requireSiteOwnerRole(w http.ResponseWriter, r *http.Request, site *domain.SiteAggregate) bool {
	if site.OwnerUserID == middleware.UserID(r) {
		return true
	}
	h.render.RenderError(w, http.StatusNotFound)
	return false
}

// SiteOverview shows one site's status, setup progress, traffic stats, and
// recent leads. It also absorbs the old single-page editor's ?tab=/&subtab=
// links (see legacyTabRedirect) so bookmarks and older emails still land on
// the right section.
func (h *Handler) SiteOverview(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)

	if r.URL.Query().Get("launched") == "1" {
		h.render.Render(w, "dashboard:launched", map[string]any{
			"Site":          site,
			"SiteURL":       h.siteURL(site.Slug),
			"EmailVerified": h.emailVerified(r),
		})
		return
	}
	if target := legacyTabRedirect(site.Slug, r.URL.Query()); target != "" {
		http.Redirect(w, r, target, http.StatusFound)
		return
	}

	leadStatus := domain.LeadStatus(r.URL.Query().Get("lead_status"))
	if leadStatus != "" && !leadStatus.Valid() {
		leadStatus = "" // ignore stale/invalid filters, fall back to all leads
	}
	leadSearch := strings.TrimSpace(r.URL.Query().Get("lead_q"))
	leadPage, _ := strconv.Atoi(r.URL.Query().Get("lead_page"))
	if leadPage < 1 {
		leadPage = 1
	}

	// The two leads queries and the analytics stats query are independent,
	// so they run concurrently rather than stacking their latencies.
	var (
		leads       []domain.Lead
		leadTotal   int
		leadCounts  domain.LeadCounts
		stats       *domain.SiteStats
		chartPoints []dailyViewPoint
		period      analyticsPeriodOpt
	)
	g, gctx := errgroup.WithContext(r.Context())
	g.Go(func() (err error) {
		leads, leadTotal, err = h.leads.ListBySiteFiltered(gctx, site.ID, leadStatus, leadSearch, leadPage)
		return
	})
	g.Go(func() (err error) {
		leadCounts, err = h.leads.Counts(gctx, site.ID)
		return
	})
	g.Go(func() error {
		stats, chartPoints, period = h.analyticsCardStats(gctx, &site.Site, r.URL.Query().Get("period"))
		return nil
	})
	if err := g.Wait(); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}

	leadTotalPages := (leadTotal + service.LeadsPageSize - 1) / service.LeadsPageSize
	checklist, checklistPercent := siteChecklist(site)
	tmpl, _ := findTemplate(site.TemplateID)

	h.renderSection(w, r, site, "", map[string]any{
		"Leads":            leads,
		"LeadCount":        leadCounts.Total,
		"NewLeadCount":     leadCounts.New,
		"LeadStatus":       leadStatus,
		"LeadSearch":       leadSearch,
		"LeadPage":         leadPage,
		"LeadTotalPages":   leadTotalPages,
		"LeadHasPrev":      leadPage > 1,
		"LeadHasNext":      leadPage < leadTotalPages,
		"LeadPrevPage":     leadPage - 1,
		"LeadNextPage":     leadPage + 1,
		"Stats":            stats,
		"ChartPoints":      chartPoints,
		"Period":           period.Key,
		"Periods":          analyticsPeriods,
		"Checklist":        checklist,
		"ChecklistPercent": checklistPercent,
		"Design":           tmpl,
	})
}

// legacyTabRedirect maps the pre-#301 editor's ?tab=/&subtab=/&csubtab=
// query params onto the section URLs that replaced them, returning "" when
// there's nothing to redirect. Old bookmarks, emails, and the browser
// history of anyone mid-session keep working.
func legacyTabRedirect(slug string, q url.Values) string {
	tab := q.Get("tab")
	if tab == "" {
		return ""
	}
	switch tab {
	case "settings":
		switch sub := q.Get("subtab"); sub {
		case "design", "form", "domain", "publishing":
			return sectionPath(slug, sub)
		default:
			target := sectionPath(slug, "content")
			if c := q.Get("csubtab"); c != "" {
				target += "#" + url.PathEscape(c)
			}
			return target
		}
	case "notifications", "access", "billing":
		return sectionPath(slug, tab)
	default: // "overview" and anything unrecognized
		return sectionPath(slug, "")
	}
}

// SiteContent renders the whole content editor — every field that appears
// on the public site — as one scrolling page of collapsible sections.
func (h *Handler) SiteContent(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	extra := map[string]any{"UploadsAvailable": h.uploads.Available()}
	for k, v := range siteContentDisplayData(site) {
		extra[k] = v
	}
	h.renderSection(w, r, site, "content", extra)
}

// SiteDesign renders the palette, brand colour, font, and design switcher.
func (h *Handler) SiteDesign(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	tmpl, _ := findTemplate(site.TemplateID)
	h.renderSection(w, r, site, "design", map[string]any{
		"Design":    tmpl,
		"Templates": siteTemplates,
		"Palettes":  tmpl.Palettes,
	})
}

// SiteForm renders the enquiry-form type picker.
func (h *Handler) SiteForm(w http.ResponseWriter, r *http.Request) {
	h.renderSection(w, r, middleware.SiteFromContext(r), "form", nil)
}

// SiteDomain renders the subdomain form and custom-domain setup. A domain
// still mid-verification gets a live Cloudflare status check on the way in —
// this is the only section that shows that status, so it's also the only
// one that pays for the check.
func (h *Handler) SiteDomain(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)

	// siteView is a private copy of the aggregate for this request to
	// render from: site itself may be the same pointer Sites' 15s aggregate
	// cache is concurrently handing to other requests, so mutating the
	// custom-domain status on it in place would race with them. See #247.
	siteView := *site

	domainData := map[string]any{
		"FallbackOrigin": h.domains.FallbackOrigin(),
		"IsPro":          siteView.Billing.IsPro(),
	}
	if siteView.CustomDomain != "" && siteView.CustomDomainStatus == domain.CustomDomainPending {
		// Best-effort: a failed Cloudflare check just means the page falls
		// back to the last known status.
		if hostname, err := h.domains.RefreshCustomDomainStatus(r.Context(), siteView.ID); err == nil {
			domainData["Hostname"] = hostname
			if hostname.Active() {
				siteView.CustomDomainStatus = domain.CustomDomainActive
			} else if hostname.Failed() {
				siteView.CustomDomainStatus = domain.CustomDomainFailed
			}
		}
	}

	h.renderSection(w, r, &siteView, "domain", map[string]any{
		"Domain":     h.cfg.Domain,
		"DomainData": domainData,
	})
}

// SitePublishing renders the publish/unpublish controls plus the share and
// export shortcuts (QR code, print one-pager, lead CSV).
func (h *Handler) SitePublishing(w http.ResponseWriter, r *http.Request) {
	h.renderSection(w, r, middleware.SiteFromContext(r), "publishing", nil)
}

// SiteNotifications renders the analytics-report email settings.
func (h *Handler) SiteNotifications(w http.ResponseWriter, r *http.Request) {
	h.renderSection(w, r, middleware.SiteFromContext(r), "notifications", nil)
}

// SiteAccess renders the teammate invite form and the current team.
func (h *Handler) SiteAccess(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.requireSiteOwnerRole(w, r, site) {
		return
	}
	members, err := h.members.List(r.Context(), site.ID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.renderSection(w, r, site, "access", map[string]any{"Members": members})
}

// SiteBilling renders the plan and subscription controls.
func (h *Handler) SiteBilling(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	if !h.requireSiteOwnerRole(w, r, site) {
		return
	}
	h.renderSection(w, r, site, "billing", nil)
}
