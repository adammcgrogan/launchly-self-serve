package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// smokeSite is the minimal site every dashboard-section smoke test renders.
func smokeSite() *domain.SiteAggregate {
	return &domain.SiteAggregate{Site: domain.Site{ID: 42, Slug: "acme", BusinessName: "Acme", TemplateID: siteTemplates[0].ID}}
}

// smokeSectionData mirrors what siteSectionData builds for every section of
// the site dashboard, so a template referencing a field the handler doesn't
// pass fails here rather than in production (LoadAll only catches parse
// errors, not execution errors from missing fields).
func smokeSectionData(site *domain.SiteAggregate, section string) map[string]any {
	var label, hint string
	for _, s := range siteSections {
		if s.Key == section {
			label, hint = s.Label, s.Hint
		}
	}
	return map[string]any{
		"Site": site, "SiteURL": "https://acme.example",
		"Section": section, "SectionLabel": label, "SectionHint": hint, "Sections": siteSections,
		"ChecklistPercent": 100, "IsOwner": true,
		"Flash": middleware.Flash{}, "CSRFToken": "tok", "EmailVerified": true,
		"Upgraded": false, "ShowTrialBanner": false, "TrialDaysLeft": 0, "ShowPastDueBanner": false,
	}
}

// TestSiteSectionsRender renders every section of the site dashboard (#301),
// each with the data its own handler supplies.
func TestSiteSectionsRender(t *testing.T) {
	chdirToRepoRoot(t)

	r := NewRenderer("launchly.ltd")
	if err := r.LoadAll(siteTemplates); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	site := smokeSite()

	extras := map[string]map[string]any{
		"": {
			"Leads": nil, "LeadCount": 7, "NewLeadCount": 0, "LeadStatus": domain.LeadStatus(""),
			"LeadSearch": "", "LeadPage": 1, "LeadTotalPages": 1,
			"Stats":  &domain.SiteStats{TotalViews: 10, UniqueVisitors: 4},
			"Period": "30", "Periods": analyticsPeriods,
			"ChartPoints": []dailyViewPoint{{Label: "Mon", Date: "1 Jan", Count: 3, HeightPx: 40}},
			"Checklist":   nil, "Design": siteTemplates[0],
		},
		"content": {"UploadsAvailable": true},
		"design":  {"Design": siteTemplates[0], "Templates": siteTemplates, "Palettes": siteTemplates[0].Palettes},
		"domain":  {"Domain": "launchly.ltd", "DomainData": map[string]any{"IsPro": false, "FallbackOrigin": "origin.launchly.ltd"}},
		"access":  {"Members": nil},
	}
	for k, v := range siteContentDisplayData(site) {
		extras["content"][k] = v
	}

	for _, s := range siteSections {
		data := smokeSectionData(site, s.Key)
		for k, v := range extras[s.Key] {
			data[k] = v
		}
		w := httptest.NewRecorder()
		r.Render(w, "dashboard:site_"+sectionTemplate(s.Key), data)
		body := w.Body.String()
		if w.Code != 200 || !strings.Contains(body, `aria-label="Site settings"`) {
			t.Errorf("section %q: render failed (status %d)", sectionTemplate(s.Key), w.Code)
		}
		// The nav must link to this section and mark it as the current page.
		if !strings.Contains(body, `href="`+sectionPath("acme", s.Key)+`"`) {
			t.Errorf("section %q: nav missing its own link", sectionTemplate(s.Key))
		}
	}
}

// TestSectionFragmentSkipsChrome checks the section-nav fetch path renders
// only the #site-workspace block — the point of it is not re-sending (and
// re-parsing) a whole document the client throws away.
func TestSectionFragmentSkipsChrome(t *testing.T) {
	chdirToRepoRoot(t)

	r := NewRenderer("launchly.ltd")
	if err := r.LoadAll(siteTemplates); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	data := smokeSectionData(smokeSite(), "form")

	full := httptest.NewRecorder()
	r.Render(full, "dashboard:site_form", data)
	frag := httptest.NewRecorder()
	r.RenderPartial(frag, "dashboard:site_form", "site_workspace", data)

	fragBody := frag.Body.String()
	if !strings.Contains(fragBody, `id="site-workspace"`) {
		t.Fatal("fragment missing #site-workspace — the client has nothing to swap in")
	}
	if !strings.Contains(fragBody, "Booking form") {
		t.Error("fragment missing the section's own content")
	}
	for _, chrome := range []string{"<!DOCTYPE html>", "app.css", "__sectionNavBound"} {
		if strings.Contains(fragBody, chrome) {
			t.Errorf("fragment still carries page chrome (%q)", chrome)
		}
	}
	if frag.Body.Len() >= full.Body.Len() {
		t.Errorf("fragment (%d bytes) is not smaller than the full page (%d bytes)", frag.Body.Len(), full.Body.Len())
	}
	t.Logf("fragment %d bytes vs full page %d bytes", frag.Body.Len(), full.Body.Len())
}

// TestAnalyticsCardPartialRenders exercises the analytics_card template both
// standalone (as the fetch-driven partial, #177) and embedded in the site
// overview page, checking they render the same period toggle.
func TestAnalyticsCardPartialRenders(t *testing.T) {
	chdirToRepoRoot(t)

	r := NewRenderer("launchly.ltd")
	if err := r.LoadAll(siteTemplates); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	site := smokeSite()
	data := map[string]any{
		"Site": site,
		"Stats": &domain.SiteStats{
			TotalViews: 10, UniqueVisitors: 4, Leads: 2,
			PrevTotalViews: 5, PrevUniqueVisitors: 4, PrevLeads: 1,
			TopPages: []domain.PageCount{{Path: "/", Count: 6}, {Path: "/contact", Count: 4}},
		},
		"ChartPoints": []dailyViewPoint{{Label: "Mon", Date: "1 Jan", Count: 3, HeightPx: 40}},
		"Period":      "30",
		"Periods":     analyticsPeriods,
		"LeadCount":   7,
	}

	w := httptest.NewRecorder()
	r.RenderPartial(w, "dashboard:analytics_card", "analytics_card", data)
	body := w.Body.String()

	if !strings.Contains(body, `id="analytics-card"`) {
		t.Error("partial missing #analytics-card container")
	}
	if !strings.Contains(body, `data-period-url="/dashboard/sites/acme/analytics-card?period=30"`) {
		t.Error("partial missing fetch URL for the 30-day period toggle")
	}
	if !strings.Contains(body, "Leads (all time)") {
		t.Error("partial missing stats content")
	}
	if !strings.Contains(body, "Top pages") || !strings.Contains(body, "/contact") {
		t.Error("partial missing top pages block")
	}
	if !strings.Contains(body, "Total conversions") {
		t.Error("partial missing total conversions row")
	}
	if !strings.Contains(body, "(+100%)") {
		t.Error("partial missing period-over-period views delta")
	}

	full := httptest.NewRecorder()
	fullData := smokeSectionData(site, "")
	for k, v := range map[string]any{
		"Leads": nil, "LeadCount": 7, "NewLeadCount": 0, "LeadStatus": domain.LeadStatus(""),
		"LeadPage": 1, "LeadTotalPages": 1, "Stats": data["Stats"], "ChartPoints": data["ChartPoints"],
		"Period": "30", "Periods": analyticsPeriods, "Checklist": nil, "Design": siteTemplates[0],
		"HoursByDay": map[time.Weekday]domain.BusinessHours{}, "Weekdays": weekdays, "Timezones": timezones,
	} {
		fullData[k] = v
	}
	r.Render(full, "dashboard:site_overview", fullData)
	if !strings.Contains(full.Body.String(), `id="analytics-card"`) {
		t.Error("overview missing embedded #analytics-card container")
	}
}
