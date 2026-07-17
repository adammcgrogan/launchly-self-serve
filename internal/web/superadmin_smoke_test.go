package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// TestSuperadminTemplatesRender exercises the superadmin dashboard and site
// templates (#181) with realistic data, checking they render the stats and
// content-edit form without panicking on template execution (LoadAll only
// catches parse errors, not execution errors from missing fields).
func TestSuperadminTemplatesRender(t *testing.T) {
	chdirToRepoRoot(t)

	r := NewRenderer()
	if err := r.LoadAll(siteTemplates); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	dashData := map[string]any{
		"Sites": []domain.Site{{ID: 1, Slug: "acme", BusinessName: "Acme", Status: domain.SiteStatusLive}},
		"Stats": domain.PlatformStats{TotalSites: 5, LiveSites: 2, DraftSites: 2, PausedSites: 1, StarterPlan: 3, ProPlan: 1, TrialingSites: 1, SignupsThisWeek: 2, SignupsThisMonth: 5},
	}
	dashW := httptest.NewRecorder()
	r.Render(dashW, "superadmin:dashboard", dashData)
	if !strings.Contains(dashW.Body.String(), "Total sites") {
		t.Error("dashboard missing stats content")
	}

	site := &domain.SiteAggregate{Site: domain.Site{ID: 42, Slug: "acme", BusinessName: "Acme", Timezone: "Europe/London"}}
	siteData := map[string]any{
		"Site":             site,
		"Leads":            []domain.Lead{},
		"SiteURL":          "https://acme.example",
		"CSRFToken":        "token",
		"Socials":          socialLinksMap(site.SocialLinks),
		"ServiceRows":      serviceRowsForDisplay(site.Services),
		"CertRows":         certificationRowsForDisplay(site.Certifications),
		"AreaRows":         serviceAreaRowsForDisplay(site.ServiceAreas),
		"Reviews":          site.Reviews,
		"TestimonialRows":  testimonialRowsForDisplay(site.Testimonials),
		"GalleryRows":      galleryRowsForDisplay(site.GalleryImages),
		"FAQRows":          faqRowsForDisplay(site.FAQItems),
		"StaffRows":        staffRowsForDisplay(site.StaffMembers),
		"HoursByDay":       businessHoursByDay(site.BusinessHours),
		"SpecialHoursRows": specialHoursRowsForDisplay(site.SpecialHours),
		"Weekdays":         weekdays,
		"Timezones":        timezones,
	}
	siteW := httptest.NewRecorder()
	r.Render(siteW, "superadmin:site", siteData)
	body := siteW.Body.String()
	if !strings.Contains(body, `action="/superadmin/sites/42/edit"`) {
		t.Error("site page missing content-edit form action")
	}
	if !strings.Contains(body, `name="business_name"`) {
		t.Error("site page missing business_name field")
	}
}
