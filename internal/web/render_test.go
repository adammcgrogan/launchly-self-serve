package web

import (
	"html/template"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRenderMidExecutionErrorFallsBackToErrorPage covers #223: a template
// that errors partway through execution (e.g. a nil deref or bad field)
// must not leave a truncated 200 response on the wire — Render should
// discard the partial output and fall back to the branded 500 error page.
func TestRenderMidExecutionErrorFallsBackToErrorPage(t *testing.T) {
	chdirToRepoRoot(t)

	rd := NewRenderer("launchly.ltd")
	if err := rd.LoadAll(nil); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	// A template that writes visible content before hitting a field that
	// doesn't exist on the data passed in, simulating a mid-render error.
	broken := template.Must(template.New("broken").Parse(
		`{{define "base"}}MARKER-BEFORE-ERROR{{.NoSuchField}}{{end}}`,
	))
	rd.tmpl["broken"] = broken

	w := httptest.NewRecorder()
	rd.Render(w, "broken", struct{ Real string }{Real: "x"})

	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "MARKER-BEFORE-ERROR") {
		t.Error("response body leaked partial template output before the error")
	}
	if !strings.Contains(w.Body.String(), "Something went wrong") {
		t.Error("response body missing branded error page content")
	}
}

// TestRenderPartialMidExecutionErrorFallsBackToErrorPage is the same check
// for RenderPartial, used by fetch-driven fragment updates.
func TestRenderPartialMidExecutionErrorFallsBackToErrorPage(t *testing.T) {
	chdirToRepoRoot(t)

	rd := NewRenderer("launchly.ltd")
	if err := rd.LoadAll(nil); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}

	broken := template.Must(template.New("broken").Parse(
		`{{define "frag"}}MARKER-BEFORE-ERROR{{.NoSuchField}}{{end}}`,
	))
	rd.tmpl["broken"] = broken

	w := httptest.NewRecorder()
	rd.RenderPartial(w, "broken", "frag", struct{ Real string }{Real: "x"})

	if w.Code != 500 {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "MARKER-BEFORE-ERROR") {
		t.Error("response body leaked partial template output before the error")
	}
	if !strings.Contains(w.Body.String(), "Something went wrong") {
		t.Error("response body missing branded error page content")
	}
}

// TestPausedBareBaseHasNoMarketingChrome covers #331: on a site's own custom
// domain the paused page is rendered against paused_base.html, which must
// not carry Launchly's marketing nav or "Start free" CTA — that is an
// acquisition funnel, and a domain the customer pays for is the wrong place
// for it. The launchly.ltd subdomain keeps the standard chrome.
func TestPausedBareBaseHasNoMarketingChrome(t *testing.T) {
	chdirToRepoRoot(t)

	rd := NewRenderer("launchly.ltd")
	if err := rd.LoadAll(nil); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	data := map[string]any{"BusinessName": "Acme Plumbing"}

	w := httptest.NewRecorder()
	rd.RenderStatus(w, "paused:bare", 503, data)
	if w.Code != 503 {
		t.Errorf("status = %d, want 503", w.Code)
	}
	bare := w.Body.String()
	for _, chrome := range []string{"Start free", "/pricing", "/templates", "cookie-consent-banner"} {
		if strings.Contains(bare, chrome) {
			t.Errorf("bare paused page leaked marketing chrome: %q", chrome)
		}
	}
	if !strings.Contains(bare, "Acme Plumbing") {
		t.Error("bare paused page missing the business name")
	}
	// The owner's way back in must point at the marketing domain: /login
	// isn't routed on a site host (SubdomainRouter sends everything to
	// ServeSite), so a relative link would loop back to this page.
	if !strings.Contains(bare, "https://launchly.ltd/login") {
		t.Error("bare paused page's owner login link is not absolute")
	}

	w = httptest.NewRecorder()
	rd.Render(w, "paused", data)
	if !strings.Contains(w.Body.String(), "Start free") {
		t.Error("subdomain paused page unexpectedly lost the standard marketing chrome")
	}
}
