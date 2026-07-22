package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

// demoSite is the hand-written seed content for one showcase demo site —
// one per template, so the /templates gallery always has a live example.
type demoSite struct {
	slug         string
	templateID   string
	palette      string
	businessName string
	tagline      string
	about        string
	ctaText      string
	contact      domain.SiteContact
}

var demoSites = []demoSite{
	{
		slug:         "demo-aurora",
		templateID:   "aurora",
		palette:      "indigo",
		businessName: "BrightPath Handyman Services",
		tagline:      "Reliable home repairs and improvements, done right the first time.",
		about:        "BrightPath has been fixing, building, and improving homes across the city for over a decade. From leaky taps to full room refreshes, our small team of tradespeople shows up on time, communicates clearly, and leaves your home cleaner than we found it.",
		ctaText:      "Get a free quote",
		contact:      domain.SiteContact{Phone: "0161 496 0142", Email: "hello@brightpathhandyman.example", Address: "14 Foundry Street, Manchester", Location: "Manchester, UK"},
	},
	{
		slug:         "demo-foundry",
		templateID:   "foundry",
		palette:      "charcoal",
		businessName: "Ironclad Plumbing & Heating",
		tagline:      "Emergency call-outs, boiler installs, and heating repairs across the city.",
		about:        "Ironclad is a Gas Safe registered plumbing and heating team covering emergency repairs, boiler servicing, and full central heating installs. We work with homeowners and landlords alike, with same-day call-outs for burst pipes and no-heat emergencies.",
		ctaText:      "Book an engineer",
		contact:      domain.SiteContact{Phone: "0113 496 0142", Email: "jobs@ironcladplumbing.example", Address: "8 Furnace Lane, Leeds", Location: "Leeds, UK"},
	},
	{
		slug:         "demo-meridian",
		templateID:   "meridian",
		palette:      "ivory",
		businessName: "Thornfield Legal Associates",
		tagline:      "Considered legal advice for individuals and small businesses.",
		about:        "Thornfield Legal Associates provides clear, straightforward legal advice on conveyancing, wills and probate, and small business contracts. We believe good advice should be plain-spoken, not dressed up in jargon — so that's how we practise.",
		ctaText:      "Book a consultation",
		contact:      domain.SiteContact{Phone: "0117 496 0142", Email: "enquiries@thornfieldlegal.example", Address: "22 Chancery Row, Bristol", Location: "Bristol, UK"},
	},
	{
		slug:         "demo-bloom",
		templateID:   "bloom",
		palette:      "blush",
		businessName: "Bloom & Co Hair Studio",
		tagline:      "A calm, considered studio for cut, colour, and care.",
		about:        "Bloom & Co is an independent hair studio built around unhurried appointments and honest advice. Whether you're after a precision cut or a full colour transformation, our stylists take the time to get it right — no rushing, no upselling.",
		ctaText:      "Book your appointment",
		contact:      domain.SiteContact{Phone: "0121 496 0142", Email: "book@bloomandco.example", Address: "5 Orchard Mews, Birmingham", Location: "Birmingham, UK"},
	},
	{
		slug:         "demo-ember",
		templateID:   "ember",
		palette:      "terracotta",
		businessName: "Ember & Oak Café",
		tagline:      "Wood-fired brunch and slow coffee in the heart of town.",
		about:        "Ember & Oak is a neighbourhood café serving wood-fired brunch, seasonal small plates, and coffee roasted five minutes down the road. We're open early for the commute crowd and stay cosy long after for something slower.",
		ctaText:      "View our menu",
		contact:      domain.SiteContact{Phone: "0141 496 0142", Email: "hello@emberandoak.example", Address: "31 Kiln Street, Glasgow", Location: "Glasgow, UK"},
	},
	{
		slug:         "demo-market",
		templateID:   "market",
		palette:      "onyx",
		businessName: "Market Row Boutique",
		tagline:      "Independent fashion and homeware, curated in-store and online.",
		about:        "Market Row Boutique stocks a rotating edit of independent clothing and homeware brands, chosen for quality over trend. Pop into our storefront or browse online — every piece is picked by the same two people who run the shop.",
		ctaText:      "Shop the collection",
		contact:      domain.SiteContact{Phone: "0131 496 0142", Email: "shop@marketrowboutique.example", Address: "19 Cobble Lane, Edinburgh", Location: "Edinburgh, UK"},
	},
	{
		slug:         "demo-surge",
		templateID:   "surge",
		palette:      "volt",
		businessName: "Surge Fitness Collective",
		tagline:      "High-energy group training and coaching that actually fits your week.",
		about:        "Surge Fitness Collective runs small-group strength and conditioning classes plus 1:1 coaching, built around real schedules rather than gym-bro ideals. New members get a free trial session before committing to a plan.",
		ctaText:      "Claim your free session",
		contact:      domain.SiteContact{Phone: "0151 496 0142", Email: "coach@surgefitness.example", Address: "3 Ironworks Yard, Liverpool", Location: "Liverpool, UK"},
	},
}

// SeedDemoSites ensures the built-in showcase demo sites exist — one per
// template, owned by ownerID — so the /templates gallery has a live example
// to link to before any real customer opts into a showcase (#39). Idempotent:
// a demo slug that already exists (e.g. seeded on a prior startup) is
// skipped, so this is safe to call on every boot.
func (s *Sites) SeedDemoSites(ctx context.Context, ownerID uuid.UUID) error {
	for _, d := range demoSites {
		existing, err := postgres.GetSiteBySlug(ctx, s.store.DB(), d.slug)
		if err != nil {
			return fmt.Errorf("check demo site %s: %w", d.slug, err)
		}
		if existing != nil {
			continue
		}
		if err := s.createDemoSite(ctx, ownerID, d); err != nil {
			return fmt.Errorf("seed demo site %s: %w", d.slug, err)
		}
		slog.Info("demo site seeded", "slug", d.slug, "template", d.templateID)
	}
	return nil
}

func (s *Sites) createDemoSite(ctx context.Context, ownerID uuid.UUID, d demoSite) error {
	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	site := &domain.Site{
		OwnerUserID:  ownerID,
		Slug:         d.slug,
		BusinessName: d.businessName,
		Tagline:      d.tagline,
		About:        d.about,
		CTAText:      d.ctaText,
		TemplateID:   d.templateID,
		Palette:      d.palette,
		Timezone:     "Europe/London",
		IsDemo:       true,
	}
	siteID, err := postgres.CreateSite(ctx, tx, site)
	if err != nil {
		return fmt.Errorf("create site: %w", err)
	}
	if err := postgres.CreateDemoSiteBilling(ctx, tx, siteID); err != nil {
		return fmt.Errorf("create billing: %w", err)
	}
	d.contact.SiteID = siteID
	if err := postgres.UpsertSiteContact(ctx, tx, &d.contact); err != nil {
		return fmt.Errorf("save contact: %w", err)
	}
	if err := postgres.UpsertSiteAnalyticsSettings(ctx, tx, &domain.SiteAnalyticsSettings{SiteID: siteID, AnalyticsFrequency: "off"}); err != nil {
		return fmt.Errorf("save analytics settings: %w", err)
	}

	return tx.Commit()
}

// DemoSiteURLSlugs returns templateID -> demo site slug for every seeded
// showcase site, for the public /templates gallery's live-example links.
func (s *Sites) DemoSiteURLSlugs(ctx context.Context) (map[string]string, error) {
	sites, err := postgres.ListDemoSites(ctx, s.store.DB())
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(sites))
	for _, site := range sites {
		out[site.TemplateID] = site.Slug
	}
	return out, nil
}
