package web

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// Renderer parses every template once at startup and caches them, matching
// the old app's pattern — server-rendered html/template, no build step.
type Renderer struct {
	tmpl map[string]*template.Template
}

func NewRenderer() *Renderer {
	return &Renderer{tmpl: make(map[string]*template.Template)}
}

var funcMap = template.FuncMap{
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
}

func (rd *Renderer) parse(key, base string, files ...string) error {
	all := append([]string{base}, files...)
	t, err := template.New(filepath.Base(base)).Funcs(funcMap).ParseFiles(all...)
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
	for _, p := range []string{"home", "pricing", "templates", "privacy", "terms", "help", "help_custom_domain", "help_address", "help_switch_template", "help_appearance", "error", "claim", "paused"} {
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
	analyticsCard := "web/templates/dashboard/analytics_card.html"
	if err := rd.parse("dashboard:site", dashBase, "web/templates/dashboard/site.html", analyticsCard); err != nil {
		return err
	}
	if err := rd.parse("dashboard:analytics_card", analyticsCard); err != nil {
		return err
	}
	if err := rd.parse("dashboard:print", "web/templates/dashboard/print.html"); err != nil {
		return err
	}

	superBase := "web/templates/superadmin/base.html"
	for _, p := range []string{"login", "dashboard", "site"} {
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

// Render executes a pre-parsed template by key, writing the result to w.
func (rd *Renderer) Render(w http.ResponseWriter, key string, data any) {
	t, ok := rd.tmpl[key]
	if !ok {
		slog.Error("render: unknown template key", "key", key)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, "base", data); err != nil {
		slog.Error("template render failed", "key", key, "error", err)
	}
}

// RenderPartial executes a named sub-template from a pre-parsed set without
// the "base" page wrapper — used for fetch-driven partial updates like the
// analytics card, which return a fragment rather than a full page.
func (rd *Renderer) RenderPartial(w http.ResponseWriter, key, name string, data any) {
	t, ok := rd.tmpl[key]
	if !ok {
		slog.Error("render: unknown template key", "key", key)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	if err := t.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("partial render failed", "key", key, "name", name, "error", err)
	}
}

// RenderError renders the branded error page with the given HTTP status code.
func (rd *Renderer) RenderError(w http.ResponseWriter, status int) {
	w.WriteHeader(status)
	t, ok := rd.tmpl["error"]
	if !ok {
		http.Error(w, "something went wrong", status)
		return
	}
	if err := t.ExecuteTemplate(w, "base", map[string]any{"Status": status}); err != nil {
		slog.Error("error template render failed", "error", err)
	}
}
