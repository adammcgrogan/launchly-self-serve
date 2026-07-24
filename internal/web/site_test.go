package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/web/middleware"
)

// testLeadSite builds a minimal live SiteAggregate suitable for exercising
// submitLeadForSite, which only reads Status, RedirectURL, and
// ThankYouMessage off the site it's handed.
func testLeadSite() *domain.SiteAggregate {
	return &domain.SiteAggregate{
		Site: domain.Site{
			ID:              1,
			Status:          domain.SiteStatusLive,
			RedirectURL:     "https://example.com/thanks",
			ThankYouMessage: "Thanks, we'll be in touch!",
		},
	}
}

func newLeadRequest(t *testing.T, form url.Values, fetch bool) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/contact", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if fetch {
		req.Header.Set("X-Requested-With", "fetch")
	}
	return req
}

// TestSubmitLeadForSiteHoneypot confirms a filled honeypot field short-circuits
// before any real validation or the leads service is touched (h.leads is left
// nil here — a real DB call would nil-panic, proving the bot path never
// reaches it), and that it looks like success to the caller either way so
// bots can't distinguish rejection from acceptance.
func TestSubmitLeadForSiteHoneypot(t *testing.T) {
	h := &Handler{}
	site := testLeadSite()

	t.Run("classic redirect", func(t *testing.T) {
		form := url.Values{"name": {"Bot"}, "website": {"http://spam.example"}}
		req := newLeadRequest(t, form, false)
		w := httptest.NewRecorder()

		h.submitLeadForSite(w, req, site, "/?lead=1")

		if w.Code != http.StatusSeeOther {
			t.Errorf("status = %d, want %d", w.Code, http.StatusSeeOther)
		}
		if loc := w.Header().Get("Location"); loc != "/?lead=1" {
			t.Errorf("redirect location = %q, want /?lead=1", loc)
		}
	})

	t.Run("fetch JSON", func(t *testing.T) {
		form := url.Values{"name": {"Bot"}, "website": {"http://spam.example"}}
		req := newLeadRequest(t, form, true)
		w := httptest.NewRecorder()

		h.submitLeadForSite(w, req, site, "/?lead=1")

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body["redirectURL"] != site.RedirectURL || body["thankYouMessage"] != site.ThankYouMessage {
			t.Errorf("body = %+v, want redirectURL/thankYouMessage from site", body)
		}
	})
}

// TestSubmitLeadForSiteMissingName confirms a blank name is rejected before
// the leads service is called (h.leads is nil here), with the error shaped
// per response mode (redirect vs. fetch JSON).
func TestSubmitLeadForSiteMissingName(t *testing.T) {
	h := &Handler{}
	site := testLeadSite()

	t.Run("classic redirect responds 400", func(t *testing.T) {
		form := url.Values{"name": {"   "}}
		req := newLeadRequest(t, form, false)
		w := httptest.NewRecorder()

		h.submitLeadForSite(w, req, site, "/?lead=1")

		if w.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
	})

	t.Run("fetch responds JSON error", func(t *testing.T) {
		form := url.Values{"name": {""}}
		req := newLeadRequest(t, form, true)
		w := httptest.NewRecorder()

		h.submitLeadForSite(w, req, site, "/?lead=1")

		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
		}
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body["error"] == "" {
			t.Error("expected non-empty error message in JSON body")
		}
	})
}

// TestSubmitLeadForSiteOversizedBody confirms a body over the 64KB cap is
// rejected as a bad request rather than being read into memory unbounded.
func TestSubmitLeadForSiteOversizedBody(t *testing.T) {
	h := &Handler{}
	site := testLeadSite()

	huge := strings.Repeat("a", 70*1024)
	form := url.Values{"name": {"Jane"}, "message": {huge}}
	req := newLeadRequest(t, form, false)
	w := httptest.NewRecorder()

	h.submitLeadForSite(w, req, site, "/?lead=1")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d for oversized body", w.Code, http.StatusBadRequest)
	}
}

// TestSubmitLeadForSlugRateLimited confirms the per-IP contact rate limiter
// rejects a submission with 429 before any site lookup happens (h.sites is
// nil here — a real lookup would nil-panic, proving the limiter check runs
// first).
func TestSubmitLeadForSlugRateLimited(t *testing.T) {
	limiter := middleware.NewRateLimiter(1, time.Minute)
	h := &Handler{contactLimiter: limiter}

	form := url.Values{"name": {"Jane"}}

	// First request consumes the only allowed slot for this IP...
	req1 := newLeadRequest(t, form, false)
	req1.RemoteAddr = "203.0.113.5:1234"
	if !limiter.Allow(middleware.ClientIP(req1)) {
		t.Fatal("test setup: expected first Allow to succeed")
	}

	// ...so the handler's own check for the same IP must reject the second.
	req2 := newLeadRequest(t, form, false)
	req2.RemoteAddr = "203.0.113.5:1234"
	w := httptest.NewRecorder()

	h.submitLeadForSlug(w, req2, "some-slug", "/?lead=1")

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}
