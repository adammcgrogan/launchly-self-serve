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

// paletteSwatchColors gives each palette a representative hex colour for the
// small preview dots in the builder wizard — cosmetic only, independent of
// Palette.CSS (which drives the actual rendered site and isn't populated
// per-palette yet).
var paletteSwatchColors = map[string]string{
	"indigo":   "#4F46E5",
	"emerald":  "#059669",
	"sunset":   "#F97316",
	"charcoal": "#1E293B",
	"rust":     "#B45309",
	"ivory":    "#CA8A04",
	"forest":   "#166534",
	"blush":    "#EC4899",
	"sage":     "#5F8D6E",
}
