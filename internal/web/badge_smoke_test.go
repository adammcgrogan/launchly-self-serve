package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

func TestSiteBadgeHiddenForProShownForStarter(t *testing.T) {
	r := NewRenderer()
	if err := r.LoadAll(siteTemplates); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	site := &domain.SiteAggregate{
		Site: domain.Site{
			Slug:         "acme",
			BusinessName: "Acme",
			TemplateID:   siteTemplates[0].ID,
		},
	}

	for _, tc := range []struct {
		plan       domain.Plan
		wantBadge  bool
	}{
		{domain.PlanStarter, true},
		{domain.PlanPro, false},
	} {
		site.Billing.Plan = tc.plan
		w := httptest.NewRecorder()
		r.Render(w, "site:"+siteTemplates[0].ID, map[string]any{"Site": site, "Socials": map[string]string{}})
		body := w.Body.String()
		hasBadge := strings.Contains(body, "Powered by")
		if hasBadge != tc.wantBadge {
			t.Errorf("plan=%s: got badge=%v, want %v", tc.plan, hasBadge, tc.wantBadge)
		}
		if tc.wantBadge && !strings.Contains(body, "utm_source=badge") {
			t.Errorf("plan=%s: badge link missing utm params", tc.plan)
		}
	}
}
