package web

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

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

func (rd *Renderer) parse(key, base string, files ...string) error {
	all := append([]string{base}, files...)
	t, err := template.ParseFiles(all...)
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
	for _, p := range []string{"home", "pricing", "templates", "privacy", "terms", "error"} {
		if err := rd.parse(p, pubBase, "web/templates/public/"+p+".html"); err != nil {
			return err
		}
	}

	authBase := "web/templates/auth/base.html"
	for _, p := range []string{"signup", "login", "forgot_password", "reset_password"} {
		if err := rd.parse("auth:"+p, authBase, "web/templates/auth/"+p+".html"); err != nil {
			return err
		}
	}

	dashBase := "web/templates/dashboard/base.html"
	for _, p := range []string{"sites", "new_site", "site", "edit", "appearance", "switch_template", "account"} {
		if err := rd.parse("dashboard:"+p, dashBase, "web/templates/dashboard/"+p+".html"); err != nil {
			return err
		}
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
