package web

import "github.com/adammcgrogan/launchly-self-serve/internal/domain"

// siteTemplates is the built-in catalog of site designs. Templates are
// compiled assets (HTML files under web/templates/sites/), not user data,
// so the catalog lives in code rather than a database table.
var siteTemplates = []domain.Template{
	{
		ID:          "aurora",
		Name:        "Aurora",
		Description: "Bright, modern, gradient-led — a confident general-purpose design.",
		Category:    "general",
		Palettes: []domain.Palette{
			{ID: "indigo", Name: "Indigo"},
			{ID: "emerald", Name: "Emerald"},
			{ID: "sunset", Name: "Sunset"},
		},
	},
	{
		ID:          "foundry",
		Name:        "Foundry",
		Description: "Bold, industrial, high-contrast — built for trades and services.",
		Category:    "general",
		Palettes: []domain.Palette{
			{ID: "charcoal", Name: "Charcoal"},
			{ID: "rust", Name: "Rust"},
		},
	},
	{
		ID:          "meridian",
		Name:        "Meridian",
		Description: "Calm, editorial, serif-led — a refined design for considered brands.",
		Category:    "general",
		Palettes: []domain.Palette{
			{ID: "ivory", Name: "Ivory"},
			{ID: "forest", Name: "Forest"},
		},
	},
	{
		ID:          "bloom",
		Name:        "Bloom",
		Description: "Soft, warm, personal — built for salons, studios, and wellness businesses.",
		Category:    "salon",
		Palettes: []domain.Palette{
			{ID: "blush", Name: "Blush"},
			{ID: "sage", Name: "Sage"},
		},
	},
}

func findTemplate(id string) (domain.Template, bool) {
	for _, t := range siteTemplates {
		if t.ID == id {
			return t, true
		}
	}
	return domain.Template{}, false
}
