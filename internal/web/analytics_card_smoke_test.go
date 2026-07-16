package web

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// TestAnalyticsCardPartialRenders exercises the analytics_card template both
// standalone (as the fetch-driven partial, #177) and embedded in the full
// dashboard:site page, checking they render the same period toggle without
// panicking on template execution (LoadAll only catches parse errors, not
// execution errors from missing fields).
func TestAnalyticsCardPartialRenders(t *testing.T) {
	chdirToRepoRoot(t)

	r := NewRenderer()
	if err := r.LoadAll(siteTemplates); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	site := &domain.SiteAggregate{Site: domain.Site{ID: 42, BusinessName: "Acme"}}
	data := map[string]any{
		"Site":        site,
		"Stats":       &domain.SiteStats{TotalViews: 10, UniqueVisitors: 4},
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
	if !strings.Contains(body, `data-period-url="/dashboard/sites/42/analytics-card?period=30"`) {
		t.Error("partial missing fetch URL for the 30-day period toggle")
	}
	if !strings.Contains(body, "Leads (all time)") {
		t.Error("partial missing stats content")
	}

	full := httptest.NewRecorder()
	fullData := map[string]any{
		"Site": site, "Leads": nil, "LeadCount": 7, "NewLeadCount": 0,
		"LeadPage": 1, "LeadTotalPages": 1, "Stats": data["Stats"], "ChartPoints": data["ChartPoints"],
		"Period": "30", "Periods": analyticsPeriods, "SiteURL": "https://acme.example",
		"Checklist": nil, "ChecklistPercent": 100, "Design": siteTemplates[0], "Templates": siteTemplates,
		"Socials": map[string]string{}, "HoursByDay": map[time.Weekday]domain.BusinessHours{}, "Weekdays": weekdays, "Timezones": timezones,
		"DomainData": map[string]any{},
	}
	r.Render(full, "dashboard:site", fullData)
	if !strings.Contains(full.Body.String(), `id="analytics-card"`) {
		t.Error("full page missing embedded #analytics-card container")
	}
}
