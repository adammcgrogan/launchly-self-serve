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
