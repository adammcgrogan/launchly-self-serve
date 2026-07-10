package web

import "net/http"

func (h *Handler) Home(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "home", map[string]any{"Templates": siteTemplates})
}

func (h *Handler) Pricing(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "pricing", map[string]any{})
}

func (h *Handler) TemplatesPage(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "templates", map[string]any{"Templates": siteTemplates})
}

func (h *Handler) Privacy(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "privacy", map[string]any{})
}

func (h *Handler) Terms(w http.ResponseWriter, r *http.Request) {
	h.render.Render(w, "terms", map[string]any{})
}
