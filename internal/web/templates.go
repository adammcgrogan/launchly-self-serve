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
	{
		ID:          "ember",
		Name:        "Ember",
		Description: "Warm, tactile, menu-led — a cosy design for restaurants, cafes, and food service.",
		Category:    "hospitality",
		Palettes: []domain.Palette{
			{ID: "terracotta", Name: "Terracotta"},
			{ID: "olive", Name: "Olive"},
		},
	},
	{
		ID:          "market",
		Name:        "Market",
		Description: "Bold, graphic, signage-led — built for retail, shops, and boutiques.",
		Category:    "retail",
		Palettes: []domain.Palette{
			{ID: "onyx", Name: "Onyx"},
			{ID: "citrus", Name: "Citrus"},
		},
	},
	{
		ID:          "surge",
		Name:        "Surge",
		Description: "High-energy, athletic, dark-led — built for gyms, studios, and fitness coaches.",
		Category:    "fitness",
		Palettes: []domain.Palette{
			{ID: "volt", Name: "Volt"},
			{ID: "cobalt", Name: "Cobalt"},
		},
	},
}

// businessType is one option in the builder wizard's "what kind of business
// is this?" picker. These are deliberately broad, general-purpose buckets —
// not a template per vertical — each just suggesting a sensible default
// design; the owner can still pick any template regardless (see issue #16).
type businessType struct {
	ID              string
	Label           string
	DefaultTemplate string
	// TemplateCategory is the Category of DefaultTemplate, backfilled in
	// init() below. The builder wizard's step 2 uses it to lead with the
	// templates suggested for the business type chosen in step 1.
	TemplateCategory string
}

var businessTypes = []businessType{
	{ID: "general", Label: "General business or trade", DefaultTemplate: "aurora"},
	{ID: "hospitality", Label: "Hospitality & food service", DefaultTemplate: "ember"},
	{ID: "retail", Label: "Retail & shop", DefaultTemplate: "market"},
	{ID: "professional", Label: "Professional services", DefaultTemplate: "meridian"},
	{ID: "fitness", Label: "Fitness & gyms", DefaultTemplate: "surge"},
	{ID: "salon", Label: "Salon, studio & wellness", DefaultTemplate: "bloom"},
}

func init() {
	for i := range businessTypes {
		if t, ok := findTemplate(businessTypes[i].DefaultTemplate); ok {
			businessTypes[i].TemplateCategory = t.Category
		}
	}
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
	"indigo":     "#4F46E5",
	"emerald":    "#059669",
	"sunset":     "#F97316",
	"charcoal":   "#1E293B",
	"rust":       "#B45309",
	"ivory":      "#CA8A04",
	"forest":     "#166534",
	"blush":      "#EC4899",
	"sage":       "#5F8D6E",
	"terracotta": "#B5502E",
	"olive":      "#4B5320",
	"onyx":       "#E11D2E",
	"citrus":     "#F59E0B",
	"volt":       "#A3E635",
	"cobalt":     "#22D3EE",
}
