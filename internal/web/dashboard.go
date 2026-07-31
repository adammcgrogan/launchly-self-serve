package web

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/sync/errgroup"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/service"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// Dashboard lists every site the logged-in user owns. A user with zero
// sites is sent straight into the builder instead — this product's whole
// promise is site-in-minutes, so there's no reason to make them find "+ New
// site" themselves.
func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	sites, err := h.sites.ListSitesByOwner(r.Context(), userID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	memberSites, err := h.members.ListSitesByMember(r.Context(), userID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	if len(sites) == 0 && len(memberSites) == 0 {
		http.Redirect(w, r, "/dashboard/sites/new", http.StatusSeeOther)
		return
	}
	h.render.Render(w, "dashboard:sites", map[string]any{
		"Sites":         sites,
		"MemberSites":   memberSites,
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

// checklistItem is one row of the site-completeness checklist shown on the
// overview section: a short label, whether it's satisfied, and a deep link
// into the editor section where the owner can complete it.
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
	base := "/dashboard/sites/" + site.Slug
	// The content editor is one scrolling page of collapsible sections, so
	// #fragment deep-links land on (and open — see the hash handling in
	// site_content.html) the exact section that needs filling in.
	items = []checklistItem{
		{Label: "Add your logo", Done: site.LogoURL != "", Link: base + "/content#basics"},
		{Label: "Write your intro (about)", Done: strings.TrimSpace(site.About) != "", Link: base + "/content#basics"},
		{Label: "List at least one service", Done: len(site.Services) > 0, Link: base + "/content#services"},
		{Label: "Add a phone number or email", Done: site.Contact.Phone != "" || site.Contact.Email != "", Link: base + "/content#contact"},
		{Label: "Set your opening hours", Done: len(site.BusinessHours) > 0, Link: base + "/content#hours"},
		{Label: "Add a photo to your gallery", Done: len(site.GalleryImages) > 0, Link: base + "/content#gallery"},
		{Label: "Publish your site", Done: site.Status == domain.SiteStatusLive, Link: base + "/publishing"},
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
// analytics_card.html hardcodes chartHeight+16px (room for the day label)
// as the chart row's fixed height — keep that in sync if this changes.
const (
	chartHeight       = 80
	chartMinBarHeight = 4
)

// analyticsPeriodOpt is one option in the analytics card's period toggle.
// Days is 0 for the "max" option (since the site was created, capped at
// maxAnalyticsPeriodDays, no daily chart).
type analyticsPeriodOpt struct {
	Key   string
	Label string
	Days  int
}

// maxAnalyticsPeriodDays caps the "max" period's lookback at the retention
// window page_views/site_events are pruned to (see analyticsRetention in
// internal/service/trialcron.go) — since() must never reach further back
// than what the pruned tables can actually still show.
const maxAnalyticsPeriodDays = 180

var analyticsPeriods = []analyticsPeriodOpt{
	{Key: "7", Label: "7 days", Days: 7},
	{Key: "30", Label: "30 days", Days: 30},
	{Key: "all", Label: "Max (180 days)", Days: 0},
}

func analyticsPeriodFromKey(key string) analyticsPeriodOpt {
	for _, p := range analyticsPeriods {
		if p.Key == key {
			return p
		}
	}
	return analyticsPeriods[0]
}

// siteAnalyticsLocation loads the site's IANA timezone, falling back to UTC
// for an unset/invalid zone — this must match GetSiteStats' own fallback
// (internal/repository/postgres/analytics.go) so the day buckets the SQL
// computes and the day keys derived from them on the Go side agree.
func siteAnalyticsLocation(timezone string) *time.Location {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.UTC
	}
	return loc
}

func (p analyticsPeriodOpt) since(siteCreatedAt time.Time) time.Time {
	if p.Days == 0 {
		cutoff := time.Now().UTC().Add(-maxAnalyticsPeriodDays * 24 * time.Hour)
		if siteCreatedAt.After(cutoff) {
			return siteCreatedAt
		}
		return cutoff
	}
	return time.Now().UTC().Add(-time.Duration(p.Days) * 24 * time.Hour)
}

// prevSince returns the start of the period immediately preceding since —
// used for the period-over-period comparison. The "Max" period (Days == 0)
// has no fixed-length prior window to compare against, so it returns since
// itself, which GetSiteStats treats as "skip the comparison".
func (p analyticsPeriodOpt) prevSince(since time.Time) time.Time {
	if p.Days == 0 {
		return since
	}
	return since.Add(-time.Duration(p.Days) * 24 * time.Hour)
}

// analyticsCardStats resolves the analytics period from a query key and
// loads that period's stats/chart data — shared by the full overview page
// and the fetch-driven analytics-card partial (SiteAnalyticsCard) so a
// period switch renders identically either way.
func (h *Handler) analyticsCardStats(ctx context.Context, site *domain.Site, periodKey string) (*domain.SiteStats, []dailyViewPoint, analyticsPeriodOpt) {
	period := analyticsPeriodFromKey(periodKey)
	since := period.since(site.CreatedAt)
	stats, _ := h.analytics.GetSiteStats(ctx, site.ID, since, period.prevSince(since), site.Timezone)
	var chartPoints []dailyViewPoint
	if stats != nil {
		loc := siteAnalyticsLocation(site.Timezone)
		if period.Days > 0 {
			chartPoints = lastNDayPoints(stats.ViewsByDay, period.Days, loc)
		} else {
			chartPoints = weeklyViewPoints(stats.ViewsByDay, since, loc)
		}
	}
	return stats, chartPoints, period
}

// SiteAnalyticsCard re-renders just the Analytics card's stats/chart for a
// new period. The period toggle on the site overview fetches this instead
// of reloading the whole dashboard page.
func (h *Handler) SiteAnalyticsCard(w http.ResponseWriter, r *http.Request) {
	site := middleware.LightSiteFromContext(r)
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

// dayKey formats t as its calendar-date key ("2006-01-02") in loc. GetSiteStats
// buckets days by the site's own timezone (each domain.DayCount.Day is the
// instant of local midnight), so callers must key/look up with the same loc
// the stats query used — keying in UTC instead would shift the date by a day
// for any site whose timezone isn't UTC.
func dayKey(t time.Time, loc *time.Location) string {
	return t.In(loc).Format("2006-01-02")
}

// lastNDayPoints turns ViewsByDay — which only has rows for days that had at
// least one view — into a dense n-day series ending today, so the chart
// always renders n bars in the right position instead of shifting to fill
// gaps. Bar heights are scaled against the period's own peak day. loc must be
// the same location GetSiteStats bucketed ViewsByDay with (the site's own
// timezone), or the day keys won't line up.
func lastNDayPoints(viewsByDay []domain.DayCount, n int, loc *time.Location) []dailyViewPoint {
	counts := make(map[string]int, len(viewsByDay))
	for _, dc := range viewsByDay {
		counts[dayKey(dc.Day, loc)] = dc.Count
	}

	now := time.Now().In(loc)
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

// weekStart returns the Monday-start of t's calendar week, at local midnight.
func weekStart(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7 // Sunday -> 7, so the week always starts on Monday
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).AddDate(0, 0, -(wd - 1))
}

// weeklyViewPoints buckets ViewsByDay into Monday-start calendar weeks from
// since to now — used for the "Max" period, where up to 180 daily bars would
// be too dense to read. Bar heights are scaled against the period's own peak
// week, mirroring lastNDayPoints. loc must be the same location GetSiteStats
// bucketed ViewsByDay with.
func weeklyViewPoints(viewsByDay []domain.DayCount, since time.Time, loc *time.Location) []dailyViewPoint {
	counts := make(map[string]int, len(viewsByDay))
	for _, dc := range viewsByDay {
		counts[dayKey(dc.Day, loc)] = dc.Count
	}

	now := time.Now().In(loc)
	var points []dailyViewPoint
	max := 0
	for start := weekStart(since.In(loc)); !start.After(now); start = start.AddDate(0, 0, 7) {
		total := 0
		for d := 0; d < 7; d++ {
			day := start.AddDate(0, 0, d)
			if day.After(now) {
				break
			}
			total += counts[day.Format("2006-01-02")]
		}
		end := start.AddDate(0, 0, 6)
		points = append(points, dailyViewPoint{
			Label: start.Format("2 Jan"),
			Date:  start.Format("2 Jan") + " – " + end.Format("2 Jan"),
			Count: total,
		})
		if total > max {
			max = total
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
	// Each site's aggregate (~17 queries) + leads list is independent of
	// every other site's, so they run concurrently rather than stacking
	// latencies sequentially for accounts with several sites.
	exportSites := make([]accountExportSite, len(sites))
	g, gctx := errgroup.WithContext(r.Context())
	for i, site := range sites {
		i, site := i, site
		g.Go(func() error {
			agg, err := h.sites.GetSiteAggregate(gctx, site.ID)
			if err != nil || agg == nil {
				if err == nil {
					err = fmt.Errorf("site %d: aggregate not found", site.ID)
				}
				return err
			}
			leads, err := h.leads.ListBySite(gctx, site.ID)
			if err != nil {
				return err
			}
			exportSites[i] = accountExportSite{SiteAggregate: agg, Leads: leads}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	export := struct {
		Profile *domain.Profile     `json:"profile"`
		Sites   []accountExportSite `json:"sites"`
	}{Profile: profile, Sites: exportSites}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="launchly-account-data.json"`)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(export)
}

// DeleteAccount permanently deletes the logged-in user's account: each site
// is deleted individually first via Sites.Delete, which cancels its Stripe
// subscription and deregisters its Cloudflare custom hostname (neither is
// reachable via the DB's cascading deletes), then the Supabase auth user is
// deleted, which cascades away the profile and anything left hanging off it.
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
		if err := h.sites.Delete(r.Context(), site.ID, service.ActorOwner); err != nil {
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
	site := middleware.LightSiteFromContext(r)
	leads, err := h.leads.ListBySite(r.Context(), site.ID)
	if err != nil {
		h.render.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-leads.csv"`, site.Slug))
	cw := csv.NewWriter(w)
	cw.Write([]string{"Name", "Email", "Phone", "Service", "Preferred time", "Party size", "Message", "Status", "Date"})
	for _, l := range leads {
		cw.Write([]string{csvSafe(l.Name), csvSafe(l.Email), csvSafe(l.Phone), csvSafe(l.ServiceLabel), csvSafe(l.PreferredTime), csvSafe(l.PartySize), csvSafe(l.Message), string(l.Status), l.CreatedAt.Format("2006-01-02 15:04")})
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		slog.Warn("lead csv export incomplete", "site_id", site.ID, "error", err)
	}
}

// ExportAnalytics downloads the site's page-view/referrer/conversion stats
// for the requested period (mirrors ExportLeads) — the "just have a
// downloadable file" option, independent of the monthly email's cadence.
func (h *Handler) ExportAnalytics(w http.ResponseWriter, r *http.Request) {
	site := middleware.LightSiteFromContext(r)
	period := analyticsPeriodFromKey(r.URL.Query().Get("period"))
	since := period.since(site.CreatedAt)
	stats, err := h.analytics.GetSiteStats(r.Context(), site.ID, since, since, site.Timezone)
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
	cw.Write([]string{"Document downloads", strconv.Itoa(stats.DocumentDownloads)})
	cw.Write([]string{"Leads", strconv.Itoa(stats.Leads)})
	cw.Write([]string{"Total conversions", strconv.Itoa(stats.TotalConversions())})
	cw.Write([]string{"Conversion rate (%)", strconv.FormatFloat(stats.ConversionRate(), 'f', 1, 64)})
	cw.Write([]string{})
	cw.Write([]string{"Day", "Views"})
	loc := siteAnalyticsLocation(site.Timezone)
	for _, d := range stats.ViewsByDay {
		cw.Write([]string{dayKey(d.Day, loc), strconv.Itoa(d.Count)})
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
	cw.Write([]string{})
	cw.Write([]string{"Page", "Views"})
	for _, p := range stats.TopPages {
		cw.Write([]string{csvSafe(p.Path), strconv.Itoa(p.Count)})
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
	site := middleware.LightSiteFromContext(r)
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
