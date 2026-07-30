package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// Renderer parses every template once at startup and caches them, matching
// the old app's pattern — server-rendered html/template, no build step.
type Renderer struct {
	tmpl    map[string]*template.Template
	funcMap template.FuncMap
}

// NewRenderer builds a Renderer whose templates can construct absolute links
// back to the marketing site (the "marketingURL" func, used by
// public/base.html's nav and footer) — needed because those templates are
// also rendered on customer subdomains, where a relative link like "/help"
// would 404 (SubdomainRouter only allowlists a handful of paths and routes
// everything else to the site handler).
func NewRenderer(domain string) *Renderer {
	fm := template.FuncMap{
		"add1": func(i int) int { return i + 1 },
		// pct returns n as a percentage of total, 0 if total isn't positive —
		// used to size proportion bars (e.g. superadmin's platform stats)
		// without every caller having to guard against a divide-by-zero.
		"pct": func(n, total int) int {
			if total <= 0 {
				return 0
			}
			return n * 100 / total
		},
		// paletteColor maps a palette ID to its representative hex swatch, so
		// marketing pages can render each template's preview in its real accent
		// colour. Falls back to the brand indigo for an unknown ID.
		"paletteColor": func(id string) string {
			if c, ok := paletteSwatchColors[id]; ok {
				return c
			}
			return "#4F46E5"
		},
		// marketingURL builds an absolute link to the marketing domain for a
		// given path, so links in shared templates are correct regardless of
		// which host (marketing domain or a customer subdomain) the current
		// page is being served from.
		"marketingURL": func(path string) string {
			return "https://" + domain + path
		},
		// asset appends a content hash to a static file's URL so a deploy
		// that changes it also changes its URL. Without this, app.css is
		// served from one unversioned path with a multi-hour max-age, and
		// everyone who visited before a deploy keeps rendering the new
		// markup against the old stylesheet until their cache expires.
		"asset": assetURL,
	}
	return &Renderer{tmpl: make(map[string]*template.Template), funcMap: fm}
}

// assetVersions caches each static file's content hash, computed on first
// use — the files don't change while the server is running.
var (
	assetVersionsMu sync.Mutex
	assetVersions   = map[string]string{}
)

// assetURL returns path with a ?v=<hash> cache buster. A file it can't read
// (which would mean the deploy is already broken) falls back to the bare
// path rather than failing the render.
func assetURL(path string) string {
	assetVersionsMu.Lock()
	defer assetVersionsMu.Unlock()
	if v, ok := assetVersions[path]; ok {
		if v == "" {
			return path
		}
		return path + "?v=" + v
	}
	var version string
	if b, err := os.ReadFile(filepath.Join("web", strings.TrimPrefix(path, "/"))); err == nil {
		sum := sha256.Sum256(b)
		version = hex.EncodeToString(sum[:])[:10]
	} else {
		slog.Warn("asset hash failed, serving unversioned", "path", path, "error", err)
	}
	assetVersions[path] = version
	if version == "" {
		return path
	}
	return path + "?v=" + version
}

func (rd *Renderer) parse(key, base string, files ...string) error {
	all := append([]string{base}, files...)
	t, err := template.New(filepath.Base(base)).Funcs(rd.funcMap).ParseFiles(all...)
	if err != nil {
		return fmt.Errorf("parse template %s: %w", key, err)
	}
	rd.tmpl[key] = t
	return nil
}

// LoadAll parses every template used by the app. Called once at startup —
// a bad template file fails the server at boot, not on first request.
func (rd *Renderer) LoadAll(templates []domain.Template) error {
	pubBase := "web/templates/public/base.html"
	for _, p := range []string{"home", "pricing", "templates", "privacy", "terms", "dpa", "help", "help_custom_domain", "help_address", "help_switch_template", "help_appearance", "error", "claim", "paused"} {
		if err := rd.parse(p, pubBase, "web/templates/public/"+p+".html"); err != nil {
			return err
		}
	}

	authBase := "web/templates/auth/base.html"
	for _, p := range []string{"signup", "login", "forgot_password", "reset_password", "resend_verification"} {
		if err := rd.parse("auth:"+p, authBase, "web/templates/auth/"+p+".html"); err != nil {
			return err
		}
	}

	dashBase := "web/templates/dashboard/base.html"
	for _, p := range []string{"sites", "new_site", "launched", "account", "accept_invite"} {
		if err := rd.parse("dashboard:"+p, dashBase, "web/templates/dashboard/"+p+".html"); err != nil {
			return err
		}
	}
	// Each section of a site's dashboard is its own page: site_layout.html
	// supplies the shared chrome (header, banners, section nav) and defines
	// "content", each site_<section>.html defines the "section" block it
	// drops into. See siteSections in site_sections.go.
	analyticsCard := "web/templates/dashboard/analytics_card.html"
	siteLayout := "web/templates/dashboard/site_layout.html"
	for _, s := range siteSections {
		name := "site_" + sectionTemplate(s.Key)
		files := []string{siteLayout, "web/templates/dashboard/" + name + ".html"}
		switch s.Key {
		case "":
			files = append(files, analyticsCard)
		case "content":
			files = append(files, "web/templates/dashboard/site_content_rows.html")
		}
		if err := rd.parse("dashboard:"+name, dashBase, files...); err != nil {
			return err
		}
	}
	if err := rd.parse("dashboard:analytics_card", analyticsCard); err != nil {
		return err
	}
	if err := rd.parse("dashboard:print", "web/templates/dashboard/print.html"); err != nil {
		return err
	}

	superBase := "web/templates/superadmin/base.html"
	for _, p := range []string{"login", "dashboard", "site", "audit"} {
		if err := rd.parse("superadmin:"+p, superBase, "web/templates/superadmin/"+p+".html"); err != nil {
			return err
		}
	}

	for _, t := range templates {
		if err := rd.parse("site:"+t.ID, "web/templates/sites/base.html", "web/templates/sites/"+t.ID+".html"); err != nil {
			return err
		}
	}

	return nil
}

// Render executes a pre-parsed template by key. It renders into a buffer
// first so a mid-execution template error (nil deref, bad field, etc.)
// never leaves a truncated page flushed to the client with a 200 — on error
// it falls back to the branded error page with a 500 instead.
func (rd *Renderer) Render(w http.ResponseWriter, key string, data any) {
	t, ok := rd.tmpl[key]
	if !ok {
		slog.Error("render: unknown template key", "key", key)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", data); err != nil {
		slog.Error("template render failed", "key", key, "error", err)
		rd.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Write(buf.Bytes())
}

// RenderPartial executes a named sub-template from a pre-parsed set without
// the "base" page wrapper — used for fetch-driven partial updates like the
// analytics card, which return a fragment rather than a full page. It
// renders into a buffer first, same as Render, so a mid-execution error
// falls back to a clean 500 instead of a truncated fragment.
func (rd *Renderer) RenderPartial(w http.ResponseWriter, key, name string, data any) {
	t, ok := rd.tmpl[key]
	if !ok {
		slog.Error("render: unknown template key", "key", key)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		slog.Error("partial render failed", "key", key, "name", name, "error", err)
		rd.RenderError(w, http.StatusInternalServerError)
		return
	}
	w.Write(buf.Bytes())
}

// RenderError renders the branded error page with the given HTTP status
// code. It renders into a buffer first so that if the error template itself
// fails to execute, the status hasn't been written yet and we can still
// fall back to a plain-text error response.
func (rd *Renderer) RenderError(w http.ResponseWriter, status int) {
	t, ok := rd.tmpl["error"]
	if !ok {
		http.Error(w, "something went wrong", status)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", map[string]any{"Status": status}); err != nil {
		slog.Error("error template render failed", "error", err)
		http.Error(w, "something went wrong", status)
		return
	}
	w.WriteHeader(status)
	w.Write(buf.Bytes())
}
