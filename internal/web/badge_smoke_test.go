package web

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// chdirToRepoRoot makes relative template paths in LoadAll resolve
// correctly: `go test` runs with the package directory as the working
// directory, but LoadAll's paths (e.g. "web/templates/...") are relative to
// the repo root, matching how the server is run in production.
func chdirToRepoRoot(t *testing.T) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

func TestSiteBadgeHiddenForProShownForStarter(t *testing.T) {
	chdirToRepoRoot(t)

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
		plan      domain.Plan
		wantBadge bool
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
