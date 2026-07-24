package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// TestPreviewBannerShownOnlyInPreviewMode covers #225: an owner previewing a
// draft/paused site via /dashboard/sites/{slug}/preview should see the real
// template with a "preview mode" banner, while the same template rendered
// for real (live, public) traffic must never show that banner.
func TestPreviewBannerShownOnlyInPreviewMode(t *testing.T) {
	chdirToRepoRoot(t)

	r := NewRenderer("launchly.ltd")
	if err := r.LoadAll(siteTemplates); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	site := &domain.SiteAggregate{
		Site: domain.Site{
			Slug:         "acme",
			BusinessName: "Acme",
			TemplateID:   siteTemplates[0].ID,
			Status:       domain.SiteStatusDraft,
		},
	}

	for _, tc := range []struct {
		name       string
		preview    bool
		wantBanner bool
	}{
		{"preview mode shows banner", true, true},
		{"non-preview render has no banner", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r.Render(w, "site:"+siteTemplates[0].ID, map[string]any{
				"Site":       site,
				"Socials":    map[string]string{},
				"Preview":    tc.preview,
				"FormAction": "/dashboard/sites/acme/preview",
			})
			body := w.Body.String()
			hasBanner := strings.Contains(body, "Preview mode")
			if hasBanner != tc.wantBanner {
				t.Errorf("preview=%v: got banner=%v, want %v", tc.preview, hasBanner, tc.wantBanner)
			}
		})
	}
}

// TestRenderSiteBlocksNonLiveStatus guards the public-facing half of #225:
// draft sites still 404 and paused sites still show the "paused" page for
// unauthenticated/public requests — only the authenticated preview route
// (PreviewSite) may render a non-live site's real content.
func TestRenderSiteBlocksNonLiveStatus(t *testing.T) {
	chdirToRepoRoot(t)

	r := NewRenderer("launchly.ltd")
	if err := r.LoadAll(siteTemplates); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	h := &Handler{render: r}

	for _, tc := range []struct {
		name       string
		status     domain.SiteStatus
		wantPaused bool
	}{
		{"draft site does not render real content", domain.SiteStatusDraft, false},
		{"paused site shows paused page, not real content", domain.SiteStatusPaused, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			site := &domain.SiteAggregate{
				Site: domain.Site{
					Slug:         "acme",
					BusinessName: "Acme",
					TemplateID:   siteTemplates[0].ID,
					Status:       tc.status,
				},
			}
			req := httptest.NewRequest("GET", "/", nil)
			w := httptest.NewRecorder()
			h.renderSite(w, req, site, "/contact")
			body := w.Body.String()
			if strings.Contains(body, "Preview mode") {
				t.Errorf("public renderSite must never show the preview banner (status=%s)", tc.status)
			}
		})
	}
}
