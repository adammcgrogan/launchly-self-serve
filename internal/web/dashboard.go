package web

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// Dashboard lists every site the logged-in user owns. A user with zero
// sites is sent straight into the builder instead — this product's whole
// promise is site-in-minutes, so there's no reason to make them find "+ New
// site" themselves.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	sites, err := h.sites.ListSitesByOwner(r.Context(), middleware.UserID(r))
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	if len(sites) == 0 {
		http.Redirect(w, r, "/dashboard/sites/new", http.StatusSeeOther)
		return
	}
	h.render.Render(w, "dashboard:sites", map[string]any{
		"Sites":         sites,
		"Flash":         middleware.GetFlash(w, r),
		"EmailVerified": h.emailVerified(r),
	})
}

// emailVerified reports whether the logged-in user has confirmed their
// email, for the dashboard's unverified-email nudge banner. It fails open
// (treats lookup errors as verified) so a profile-lookup hiccup never blocks
// the dashboard from rendering.
func (h *Handler) emailVerified(r *http.Request) bool {
	profile, err := h.accounts.GetProfile(r.Context(), middleware.UserID(r))
	if err != nil || profile == nil {
		return true
	}
	return profile.EmailVerified
}

// SiteOverview shows one site's status, live URL, trial/billing state,
// stats, and recent leads, plus every site-level setting grouped into tabs.
// RequireSiteOwner has already loaded the site into the request context.
func (h *Handler) SiteOverview(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)

	if r.URL.Query().Get("launched") == "1" {
		siteURL := h.siteURL(site.Slug)
		h.render.Render(w, "dashboard:launched", map[string]any{
			"Site":          site,
			"SiteURL":       siteURL,
			"EmailVerified": h.emailVerified(r),
		})
		return
	}

	leadStatus := domain.LeadStatus(r.URL.Query().Get("lead_status"))
	leadSearch := strings.TrimSpace(r.URL.Query().Get("lead_q"))
	leadPage, _ := strconv.Atoi(r.URL.Query().Get("lead_page"))
	if leadPage < 1 {
		leadPage = 1
	}
	leads, leadTotal, err := h.leads.ListBySiteFiltered(r.Context(), site.ID, leadStatus, leadSearch, leadPage)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	leadCounts, err := h.leads.Counts(r.Context(), site.ID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	leadTotalPages := (leadTotal + service.LeadsPageSize - 1) / service.LeadsPageSize
	stats, chartPoints, period := h.analyticsCardStats(r.Context(), site, r.URL.Query().Get("period"))

	tmpl, _ := findTemplate(site.TemplateID)
	checklist, checklistPercent := siteChecklist(site)

	domainData := map[string]any{
		"FallbackOrigin": h.domains.FallbackOrigin(),
		"IsPro":          site.Billing.Plan == domain.PlanPro,
	}
	if site.CustomDomain != "" && site.CustomDomainStatus == domain.CustomDomainPending {
		if hostname, err := h.domains.RefreshCustomDomainStatus(r.Context(), site.ID); err == nil {
			domainData["Hostname"] = hostname
			if hostname.Active() {
				site.CustomDomainStatus = domain.CustomDomainActive
			} else if hostname.Failed() {
				site.CustomDomainStatus = domain.CustomDomainFailed
			}
		}
	}

	h.render.Render(w, "dashboard:site", map[string]any{
		"Site":           site,
		"Leads":          leads,
		"LeadCount":      leadCounts.Total,
		"NewLeadCount":   leadCounts.New,
		"LeadStatus":     leadStatus,
		"LeadSearch":     leadSearch,
		"LeadPage":       leadPage,
		"LeadTotalPages": leadTotalPages,
		"LeadHasPrev":    leadPage > 1,
		"LeadHasNext":    leadPage < leadTotalPages,
		"LeadPrevPage":   leadPage - 1,
		"LeadNextPage":   leadPage + 1,
		"Stats":          stats,
		"ChartPoints":    chartPoints,
		"Period":         period.Key,
		"Periods":        analyticsPeriods,
		"SiteURL":        h.siteURL(site.Slug),
		"Flash":          middleware.GetFlash(w, r),
		"CSRFToken":      h.csrf.Token(middleware.UserID(r).String(), h.auth.SessionNonce(r)),
		"Upgraded":       r.URL.Query().Get("upgraded") == "1",
		"EmailVerified":  h.emailVerified(r),

		"Checklist":        checklist,
		"ChecklistPercent": checklistPercent,

		"Design":           tmpl,
		"Templates":        siteTemplates,
		"Palettes":         tmpl.Palettes,
		"Socials":          socialLinksMap(site.SocialLinks),
		"ServiceRows":      serviceRowsForDisplay(site.Services),
		"CertsText":        certificationsToLines(site.Certifications),
		"ServiceAreasText": serviceAreasToLines(site.ServiceAreas),
		"Reviews":          site.Reviews,
		"TestimonialsText": testimonialsToLines(site.Testimonials),
		"GalleryText":      galleryToLines(site.GalleryImages),
		"FAQRows":          faqRowsForDisplay(site.FAQItems),
		"StaffRows":        staffRowsForDisplay(site.StaffMembers),
		"HoursByDay":       businessHoursByDay(site.BusinessHours),
		"SpecialHoursRows": specialHoursRowsForDisplay(site.SpecialHours),
		"Weekdays":         weekdays,
		"Timezones":        timezones,
		"Domain":           h.cfg.Domain,
		"DomainData":       domainData,
		"UploadsAvailable": h.uploads.Available(),
	})
}

// checklistItem is one row of the site-completeness checklist shown on the
// overview tab: a short label, whether it's satisfied, and a deep link into
// the editor sub-tab where the owner can complete it.
type checklistItem struct {
	Label string
	Done  bool
	Link  string
}

// siteChecklist scores how complete a site is from its already-loaded
// aggregate, returning the checklist rows and the percentage done. It nudges
// a new owner toward the handful of things that most improve a site's
// conversion — a logo, an intro, services, hours, contact details, and
// actually publishing — without needing any extra queries.
func siteChecklist(site *domain.SiteAggregate) (items []checklistItem, percent int) {
	base := fmt.Sprintf("/dashboard/sites/%d?tab=settings&subtab=", site.ID)
	items = []checklistItem{
		{Label: "Add your logo", Done: site.LogoURL != "", Link: base + "content"},
		{Label: "Write your intro (about)", Done: strings.TrimSpace(site.About) != "", Link: base + "content"},
		{Label: "List at least one service", Done: len(site.Services) > 0, Link: base + "content"},
		{Label: "Add a phone number or email", Done: site.Contact.Phone != "" || site.Contact.Email != "", Link: base + "content"},
		{Label: "Set your opening hours", Done: len(site.BusinessHours) > 0, Link: base + "content"},
		{Label: "Add a photo to your gallery", Done: len(site.GalleryImages) > 0, Link: base + "content"},
		{Label: "Publish your site", Done: site.Status == domain.SiteStatusLive, Link: base + "publishing"},
	}
	done := 0
	for _, it := range items {
		if it.Done {
			done++
		}
	}
	return items, done * 100 / len(items)
}

// dailyViewPoint is one bar in the 7-day page-views chart on the site
// overview: a day label/date and its view count, plus a precomputed bar
// height so the template does no charting math.
type dailyViewPoint struct {
	Label    string // weekday, e.g. "Mon"
	Date     string // e.g. "9 Jul"
	Count    int
	HeightPx int
}

// chartHeight and chartMinBarHeight size the 7-day page-views chart's bars —
// kept small since this is a compact dashboard card, not a full chart page.
// dashboard/site.html hardcodes chartHeight+16px (room for the day label) as
// the chart row's fixed height — keep that in sync if this changes.
const (
	chartHeight       = 80
	chartMinBarHeight = 4
)

// analyticsPeriodOpt is one option in the analytics card's period toggle.
// Days is 0 for "all time" (since the site was created, no daily chart).
type analyticsPeriodOpt struct {
	Key   string
	Label string
	Days  int
}

var analyticsPeriods = []analyticsPeriodOpt{
	{Key: "7", Label: "7 days", Days: 7},
	{Key: "30", Label: "30 days", Days: 30},
	{Key: "all", Label: "All time", Days: 0},
}

func analyticsPeriodFromKey(key string) analyticsPeriodOpt {
	for _, p := range analyticsPeriods {
		if p.Key == key {
			return p
		}
	}
	return analyticsPeriods[0]
}

func (p analyticsPeriodOpt) since(siteCreatedAt time.Time) time.Time {
	if p.Days == 0 {
		return siteCreatedAt
	}
	return time.Now().UTC().Add(-time.Duration(p.Days) * 24 * time.Hour)
}

// analyticsCardStats resolves the analytics period from a query key and
// loads that period's stats/chart data — shared by the full overview page
// and the fetch-driven analytics-card partial (SiteAnalyticsCard) so a
// period switch renders identically either way.
func (h *Handler) analyticsCardStats(ctx context.Context, site *domain.SiteAggregate, periodKey string) (*domain.SiteStats, []dailyViewPoint, analyticsPeriodOpt) {
	period := analyticsPeriodFromKey(periodKey)
	stats, _ := h.analytics.GetSiteStats(ctx, site.ID, period.since(site.CreatedAt), site.Timezone)
	var chartPoints []dailyViewPoint
	if stats != nil && period.Days > 0 {
		chartPoints = lastNDayPoints(stats.ViewsByDay, period.Days)
	}
	return stats, chartPoints, period
}

// SiteAnalyticsCard re-renders just the Analytics card's stats/chart for a
// new period. The period toggle in dashboard:site fetches this instead of
// reloading the whole dashboard page.
func (h *Handler) SiteAnalyticsCard(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	stats, chartPoints, period := h.analyticsCardStats(r.Context(), site, r.URL.Query().Get("period"))
	leadCounts, err := h.leads.Counts(r.Context(), site.ID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.render.RenderPartial(w, "dashboard:analytics_card", "analytics_card", map[string]any{
		"Site":        site,
		"Stats":       stats,
		"ChartPoints": chartPoints,
		"Period":      period.Key,
		"Periods":     analyticsPeriods,
		"LeadCount":   leadCounts.Total,
	})
}

// lastNDayPoints turns ViewsByDay — which only has rows for days that had at
// least one view — into a dense n-day series ending today, so the chart
// always renders n bars in the right position instead of shifting to fill
// gaps. Bar heights are scaled against the period's own peak day.
func lastNDayPoints(viewsByDay []domain.DayCount, n int) []dailyViewPoint {
	counts := make(map[string]int, len(viewsByDay))
	for _, dc := range viewsByDay {
		counts[dc.Day.UTC().Format("2006-01-02")] = dc.Count
	}

	now := time.Now().UTC()
	points := make([]dailyViewPoint, n)
	max := 0
	for i := range points {
		day := now.AddDate(0, 0, -(n - 1 - i))
		count := counts[day.Format("2006-01-02")]
		points[i] = dailyViewPoint{Label: day.Format("Mon"), Date: day.Format("2 Jan"), Count: count}
		if count > max {
			max = count
		}
	}
	if max == 0 {
		return points
	}
	for i := range points {
		if points[i].Count == 0 {
			continue
		}
		h := points[i].Count * chartHeight / max
		if h < chartMinBarHeight {
			h = chartMinBarHeight
		}
		points[i].HeightPx = h
	}
	return points
}

// Account shows the logged-in user's email and account-level actions
// (password reset goes through Supabase's own recovery email flow).
func (h *Handler) Account(w http.ResponseWriter, r *http.Request) {
	profile, err := h.accounts.GetProfile(r.Context(), middleware.UserID(r))
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	sites, err := h.sites.ListSitesByOwner(r.Context(), middleware.UserID(r))
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	initial := "?"
	if profile.Email != "" {
		initial = strings.ToUpper(profile.Email[:1])
	}
	h.render.Render(w, "dashboard:account", map[string]any{
		"Profile":       profile,
		"Initial":       initial,
		"Sites":         sites,
		"Flash":         middleware.GetFlash(w, r),
		"EmailVerified": profile.EmailVerified,
		"CSRFToken":     h.csrf.Token(middleware.UserID(r).String(), h.auth.SessionNonce(r)),
	})
}

// accountExportSite bundles a site's full aggregate with its leads for the
// account data export — leads aren't part of SiteAggregate since most
// callers (e.g. the site editor) don't need them alongside every field.
type accountExportSite struct {
	*domain.SiteAggregate
	Leads []domain.Lead `json:"leads"`
}

// ExportAccountData downloads everything this app stores about the logged-in
// user — their profile, every site they own, and each site's leads — as JSON.
func (h *Handler) ExportAccountData(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	profile, err := h.accounts.GetProfile(r.Context(), userID)
	if err != nil || profile == nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	sites, err := h.sites.ListSitesByOwner(r.Context(), userID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	export := struct {
		Profile *domain.Profile     `json:"profile"`
		Sites   []accountExportSite `json:"sites"`
	}{Profile: profile, Sites: []accountExportSite{}}
	for _, site := range sites {
		agg, err := h.sites.GetSiteAggregate(r.Context(), site.ID)
		if err != nil || agg == nil {
			h.render.RenderError(w, http.StatusInternalServerError)
			return
		}
		leads, err := h.leads.ListBySite(r.Context(), site.ID)
		if err != nil {
			h.render.RenderError(w, http.StatusInternalServerError)
			return
		}
		export.Sites = append(export.Sites, accountExportSite{SiteAggregate: agg, Leads: leads})
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="launchly-account-data.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(export)
}

// DeleteAccount permanently deletes the logged-in user's account: any Stripe
// subscriptions on their sites are cancelled first (Stripe isn't reachable
// via the DB's cascading deletes), then the Supabase auth user is deleted,
// which cascades away the profile, sites, and everything hanging off them.
func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	if !h.checkCSRF(w, r, userID.String(), h.auth.SessionNonce(r)) {
		return
	}
	sites, err := h.sites.ListSitesByOwner(r.Context(), userID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	for _, site := range sites {
		if err := h.billing.CancelSubscriptionIfActive(r.Context(), site.ID); err != nil {
			h.render.RenderError(w, http.StatusInternalServerError)
			return
		}
	}
	if err := h.accounts.DeleteAccount(r.Context(), userID); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	h.auth.ClearSessionCookies(w)
	http.Redirect(w, r, "/", http.StatusSeeOther)
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
	cw.Write([]string{"Name", "Email", "Phone", "Service", "Preferred time", "Message", "Status", "Date"})
	for _, l := range leads {
		cw.Write([]string{csvSafe(l.Name), csvSafe(l.Email), csvSafe(l.Phone), csvSafe(l.ServiceLabel), csvSafe(l.PreferredTime), csvSafe(l.Message), string(l.Status), l.CreatedAt.Format("2006-01-02 15:04")})
	}
	cw.Flush()
}

// ExportAnalytics downloads the site's page-view/referrer/conversion stats
// for the requested period (mirrors ExportLeads) — the "just have a
// downloadable file" option, independent of the monthly email's cadence.
func (h *Handler) ExportAnalytics(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	period := analyticsPeriodFromKey(r.URL.Query().Get("period"))
	stats, err := h.analytics.GetSiteStats(r.Context(), site.ID, period.since(site.CreatedAt), site.Timezone)
	if err != nil || stats == nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-analytics-%s.csv"`, site.Slug, period.Key))
	cw := csv.NewWriter(w)
	cw.Write([]string{"Metric", "Value"})
	cw.Write([]string{"Total views", strconv.Itoa(stats.TotalViews)})
	cw.Write([]string{"Unique visitors", strconv.Itoa(stats.UniqueVisitors)})
	cw.Write([]string{"Call taps", strconv.Itoa(stats.CallTaps)})
	cw.Write([]string{"WhatsApp taps", strconv.Itoa(stats.WhatsAppTaps)})
	cw.Write([]string{"Directions clicks", strconv.Itoa(stats.DirectionsClicks)})
	cw.Write([]string{"Leads", strconv.Itoa(stats.Leads)})
	cw.Write([]string{})
	cw.Write([]string{"Day", "Views"})
	for _, d := range stats.ViewsByDay {
		cw.Write([]string{d.Day.Format("2006-01-02"), strconv.Itoa(d.Count)})
	}
	cw.Write([]string{})
	cw.Write([]string{"Referrer", "Views"})
	for _, ref := range stats.TopReferrers {
		label := ref.Referrer
		if label == "" {
			label = "Direct"
		}
		cw.Write([]string{csvSafe(label), strconv.Itoa(ref.Count)})
	}
	cw.Flush()
}

// csvSafe neutralises CSV formula injection: visitor-controlled lead fields
// are attacker input, and a leading =, +, -, @, tab, or CR makes Excel/Sheets
// evaluate the cell as a formula when the owner opens their export.
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// SiteQRCode renders a PNG QR code encoding the site's public URL, for the
// owner to download and use in offline marketing (van livery, flyers, etc).
func (h *Handler) SiteQRCode(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	png, err := qrcode.Encode(h.siteURL(site.Slug), qrcode.Medium, 512)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-qr.png"`, site.Slug))
	w.Write(png)
}

// SitePrintPage renders a print-ready one-pager (logo, business name,
// services, hours, QR code) the owner can print or save as a PDF straight
// from the browser — no server-side PDF dependency needed.
func (h *Handler) SitePrintPage(w http.ResponseWriter, r *http.Request) {
	site := middleware.SiteFromContext(r)
	h.render.Render(w, "dashboard:print", map[string]any{
		"Site":    site,
		"SiteURL": h.siteURL(site.Slug),
	})
}
