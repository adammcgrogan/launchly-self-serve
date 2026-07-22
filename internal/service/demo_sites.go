package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

// demoSite is the hand-written seed content for one showcase demo site —
// one per template, so the /templates gallery always has a live example
// that shows off every content section the builder supports (services,
// testimonials, gallery, FAQ, staff, hours, reviews, socials).
type demoSite struct {
	slug         string
	templateID   string
	palette      string
	formType     domain.FormType
	businessName string
	tagline      string
	about        string
	ctaText      string
	contact      domain.SiteContact

	services       []domain.Service
	certifications []domain.Certification
	testimonials   []domain.Testimonial
	gallery        []domain.GalleryImage
	faqItems       []domain.FAQItem
	staff          []domain.StaffMember
	hours          []domain.BusinessHours
	serviceAreas   []domain.ServiceArea
	social         []domain.SocialLink
	reviews        domain.SiteReviews
}

// weekdayHours builds a Mon-Fri OpensAt/ClosesAt schedule plus a shorter
// Saturday and a closed Sunday — a realistic default week for the demo
// sites below.
func weekdayHours(opens, closes, satOpens, satCloses string) []domain.BusinessHours {
	hours := make([]domain.BusinessHours, 7)
	for wd := 0; wd <= 6; wd++ {
		h := domain.BusinessHours{Weekday: time.Weekday(wd)}
		switch wd {
		case 0: // Sunday
			h.Closed = true
		case 6: // Saturday
			if satOpens == "" {
				h.Closed = true
			} else {
				h.OpensAt, h.ClosesAt = satOpens, satCloses
			}
		default:
			h.OpensAt, h.ClosesAt = opens, closes
		}
		hours[wd] = h
	}
	return hours
}

var demoSites = []demoSite{
	{
		slug:         "demo-aurora",
		templateID:   "aurora",
		palette:      "indigo",
		formType:     domain.FormTypeContact,
		businessName: "BrightPath Handyman Services",
		tagline:      "Reliable home repairs and improvements, done right the first time.",
		about:        "BrightPath has been fixing, building, and improving homes across the city for over a decade. From leaky taps to full room refreshes, our small team of tradespeople shows up on time, communicates clearly, and leaves your home cleaner than we found it.",
		ctaText:      "Get a free quote",
		contact:      domain.SiteContact{Phone: "0161 496 0142", Email: "hello@brightpathhandyman.example", Address: "14 Foundry Street, Manchester", Location: "Manchester, UK", MapURL: "https://maps.google.com/?q=Manchester"},
		services: []domain.Service{
			{Label: "General repairs", Description: "Odd jobs, snagging lists, and small fixes around the home.", PriceText: "from £45"},
			{Label: "Room refresh", Description: "Painting, flooring, and fixtures for a single room.", PriceText: "from £350"},
			{Label: "Flat-pack & fitting", Description: "Furniture assembly and shelving/curtain fitting.", PriceText: "from £30"},
		},
		certifications: []domain.Certification{{Label: "Fully insured"}, {Label: "DBS checked"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Rachel M.", AuthorRole: "Homeowner", Quote: "Turned up exactly on time and fixed three jobs in one visit. Couldn't be easier."},
			{AuthorName: "Tom D.", AuthorRole: "Landlord", Quote: "My go-to for tenant call-outs — quick, tidy, and fair pricing."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/brightpath-1/800/600", AltText: "Freshly painted living room"},
			{URL: "https://picsum.photos/seed/brightpath-2/800/600", AltText: "Handyman fitting a shelf"},
			{URL: "https://picsum.photos/seed/brightpath-3/800/600", AltText: "Toolbox and equipment"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do you offer free quotes?", Answer: "Yes — send us a few photos and we'll give you a ballpark price before booking."},
			{Question: "What areas do you cover?", Answer: "Manchester and the surrounding boroughs, usually within a 15-mile radius."},
		},
		staff: []domain.StaffMember{
			{Name: "Dave Ellison", Role: "Founder & Handyman", PhotoURL: "https://picsum.photos/seed/brightpath-dave/400/400", Bio: "10+ years fixing homes across Greater Manchester."},
		},
		hours:        weekdayHours("08:00", "17:30", "09:00", "13:00"),
		serviceAreas: []domain.ServiceArea{{Area: "Manchester"}, {Area: "Salford"}, {Area: "Stockport"}},
		social:       []domain.SocialLink{{Platform: domain.SocialFacebook, URL: "https://facebook.com/brightpathhandyman"}, {Platform: domain.SocialInstagram, URL: "https://instagram.com/brightpathhandyman"}},
		reviews:      domain.SiteReviews{Rating: "4.9", ReviewCount: 62, ReviewURL: "https://google.com/search?q=brightpath+handyman+reviews"},
	},
	{
		slug:         "demo-foundry",
		templateID:   "foundry",
		palette:      "charcoal",
		formType:     domain.FormTypeContact,
		businessName: "Ironclad Plumbing & Heating",
		tagline:      "Emergency call-outs, boiler installs, and heating repairs across the city.",
		about:        "Ironclad is a Gas Safe registered plumbing and heating team covering emergency repairs, boiler servicing, and full central heating installs. We work with homeowners and landlords alike, with same-day call-outs for burst pipes and no-heat emergencies.",
		ctaText:      "Book an engineer",
		contact:      domain.SiteContact{Phone: "0113 496 0142", Email: "jobs@ironcladplumbing.example", Address: "8 Furnace Lane, Leeds", Location: "Leeds, UK", MapURL: "https://maps.google.com/?q=Leeds"},
		services: []domain.Service{
			{Label: "Emergency call-out", Description: "Burst pipes, leaks, and no-heat emergencies, same day.", PriceText: "from £80"},
			{Label: "Boiler service", Description: "Annual safety check and service for gas boilers.", PriceText: "£90"},
			{Label: "Full heating install", Description: "New central heating systems, quoted on survey.", PriceText: "from £2,800"},
		},
		certifications: []domain.Certification{{Label: "Gas Safe registered"}, {Label: "Worcester Bosch accredited"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Priya S.", AuthorRole: "Homeowner", Quote: "Boiler went out on a Sunday and they had an engineer out within two hours."},
			{AuthorName: "Mark H.", AuthorRole: "Property Manager", Quote: "We use Ironclad across all our rental properties — always professional."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/ironclad-1/800/600", AltText: "Engineer servicing a boiler"},
			{URL: "https://picsum.photos/seed/ironclad-2/800/600", AltText: "Central heating pipework"},
			{URL: "https://picsum.photos/seed/ironclad-3/800/600", AltText: "Ironclad van on site"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Are you available for emergencies?", Answer: "Yes, we run a 24/7 emergency call-out line for burst pipes and no-heat situations."},
			{Question: "Do you offer fixed-price quotes?", Answer: "For installs and services, yes — we survey first and confirm a fixed price before starting."},
		},
		staff: []domain.StaffMember{
			{Name: "Karl Iverson", Role: "Lead Gas Engineer", PhotoURL: "https://picsum.photos/seed/ironclad-karl/400/400", Bio: "Gas Safe registered for 15 years, specialising in combi boiler installs."},
			{Name: "Josh Fenwick", Role: "Plumbing Engineer", PhotoURL: "https://picsum.photos/seed/ironclad-josh/400/400", Bio: "Handles emergency call-outs and bathroom installs."},
		},
		hours:        weekdayHours("07:00", "18:00", "08:00", "14:00"),
		serviceAreas: []domain.ServiceArea{{Area: "Leeds"}, {Area: "Bradford"}, {Area: "Wakefield"}},
		social:       []domain.SocialLink{{Platform: domain.SocialFacebook, URL: "https://facebook.com/ironcladplumbing"}},
		reviews:      domain.SiteReviews{Rating: "4.8", ReviewCount: 134, ReviewURL: "https://google.com/search?q=ironclad+plumbing+reviews"},
	},
	{
		slug:         "demo-meridian",
		templateID:   "meridian",
		palette:      "ivory",
		formType:     domain.FormTypeContact,
		businessName: "Thornfield Legal Associates",
		tagline:      "Considered legal advice for individuals and small businesses.",
		about:        "Thornfield Legal Associates provides clear, straightforward legal advice on conveyancing, wills and probate, and small business contracts. We believe good advice should be plain-spoken, not dressed up in jargon — so that's how we practise.",
		ctaText:      "Book a consultation",
		contact:      domain.SiteContact{Phone: "0117 496 0142", Email: "enquiries@thornfieldlegal.example", Address: "22 Chancery Row, Bristol", Location: "Bristol, UK", MapURL: "https://maps.google.com/?q=Bristol"},
		services: []domain.Service{
			{Label: "Residential conveyancing", Description: "Buying or selling a home, start to completion.", PriceText: "from £750"},
			{Label: "Wills & probate", Description: "Straightforward wills and probate administration.", PriceText: "from £250"},
			{Label: "Small business contracts", Description: "Terms of service, supplier agreements, and NDAs.", PriceText: "from £400"},
		},
		certifications: []domain.Certification{{Label: "Law Society regulated"}, {Label: "Lexcel accredited"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Alison P.", AuthorRole: "First-time buyer", Quote: "Explained every step in plain English — made a stressful process feel manageable."},
			{AuthorName: "Grant & Co", AuthorRole: "Small business client", Quote: "Sorted our supplier contracts quickly and at a fraction of the cost we expected."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/thornfield-1/800/600", AltText: "Meeting room at Thornfield Legal"},
			{URL: "https://picsum.photos/seed/thornfield-2/800/600", AltText: "Reception desk"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do you offer a free initial consultation?", Answer: "Yes, the first 20 minutes of any new matter is free of charge."},
			{Question: "Do you handle cases outside Bristol?", Answer: "Most of our work is done remotely, so we act for clients across England and Wales."},
		},
		staff: []domain.StaffMember{
			{Name: "Eleanor Thornfield", Role: "Founding Solicitor", PhotoURL: "https://picsum.photos/seed/thornfield-eleanor/400/400", Bio: "20 years' experience in residential property and private client law."},
		},
		hours:   weekdayHours("09:00", "17:30", "", ""),
		social:  []domain.SocialLink{{Platform: domain.SocialLinkedIn, URL: "https://linkedin.com/company/thornfield-legal"}},
		reviews: domain.SiteReviews{Rating: "4.9", ReviewCount: 41, ReviewURL: "https://google.com/search?q=thornfield+legal+reviews"},
	},
	{
		slug:         "demo-bloom",
		templateID:   "bloom",
		palette:      "blush",
		formType:     domain.FormTypeBooking,
		businessName: "Bloom & Co Hair Studio",
		tagline:      "A calm, considered studio for cut, colour, and care.",
		about:        "Bloom & Co is an independent hair studio built around unhurried appointments and honest advice. Whether you're after a precision cut or a full colour transformation, our stylists take the time to get it right — no rushing, no upselling.",
		ctaText:      "Book your appointment",
		contact:      domain.SiteContact{Phone: "0121 496 0142", Email: "book@bloomandco.example", Address: "5 Orchard Mews, Birmingham", Location: "Birmingham, UK", MapURL: "https://maps.google.com/?q=Birmingham"},
		services: []domain.Service{
			{Label: "Cut & finish", Description: "Consultation, wash, cut, and blow-dry.", PriceText: "from £42"},
			{Label: "Full colour", Description: "Root-to-tip colour with gloss finish.", PriceText: "from £95"},
			{Label: "Balayage", Description: "Hand-painted highlights for a natural, sun-kissed look.", PriceText: "from £130"},
		},
		certifications: []domain.Certification{{Label: "L'Oréal Colour Specialist"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Chloe B.", AuthorRole: "Regular client", Quote: "Best balayage I've ever had, and the studio itself is so relaxing."},
			{AuthorName: "Nisha R.", AuthorRole: "New client", Quote: "Booked online in seconds and my stylist really listened to what I wanted."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/bloomandco-1/800/600", AltText: "Hair studio interior"},
			{URL: "https://picsum.photos/seed/bloomandco-2/800/600", AltText: "Balayage colour result"},
			{URL: "https://picsum.photos/seed/bloomandco-3/800/600", AltText: "Styling station"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do I need a patch test before colour?", Answer: "Yes, for any first-time colour appointment we require a patch test 48 hours beforehand."},
			{Question: "Can I book online?", Answer: "Yes — use the booking button above to pick a stylist and time that suits you."},
		},
		staff: []domain.StaffMember{
			{Name: "Amara Bloom", Role: "Founder & Colourist", PhotoURL: "https://picsum.photos/seed/bloomandco-amara/400/400", Bio: "Specialises in balayage and colour correction."},
			{Name: "Freya Lund", Role: "Senior Stylist", PhotoURL: "https://picsum.photos/seed/bloomandco-freya/400/400", Bio: "Cutting specialist with a focus on low-maintenance styles."},
		},
		hours:   weekdayHours("09:30", "18:00", "09:00", "16:00"),
		social:  []domain.SocialLink{{Platform: domain.SocialInstagram, URL: "https://instagram.com/bloomandcostudio"}, {Platform: domain.SocialTikTok, URL: "https://tiktok.com/@bloomandcostudio"}},
		reviews: domain.SiteReviews{Rating: "5.0", ReviewCount: 87, ReviewURL: "https://google.com/search?q=bloom+and+co+hair+studio+reviews"},
	},
	{
		slug:         "demo-ember",
		templateID:   "ember",
		palette:      "terracotta",
		formType:     domain.FormTypeContact,
		businessName: "Ember & Oak Café",
		tagline:      "Wood-fired brunch and slow coffee in the heart of town.",
		about:        "Ember & Oak is a neighbourhood café serving wood-fired brunch, seasonal small plates, and coffee roasted five minutes down the road. We're open early for the commute crowd and stay cosy long after for something slower.",
		ctaText:      "View our menu",
		contact:      domain.SiteContact{Phone: "0141 496 0142", Email: "hello@emberandoak.example", Address: "31 Kiln Street, Glasgow", Location: "Glasgow, UK", MapURL: "https://maps.google.com/?q=Glasgow"},
		services: []domain.Service{
			{Label: "Wood-fired brunch", Description: "Served all day, weekends till 3pm.", PriceText: "from £9"},
			{Label: "Seasonal small plates", Description: "Changing weekly with local produce.", PriceText: "from £7"},
			{Label: "Private bookings", Description: "Our back room seats up to 20 for events.", PriceText: "on request"},
		},
		testimonials: []domain.Testimonial{
			{AuthorName: "Callum W.", AuthorRole: "Regular", Quote: "Best flat white in the city and the sourdough is unreal."},
			{AuthorName: "Sana K.", AuthorRole: "First visit", Quote: "Cosy, unpretentious, and the wood-fired eggs were worth the trip alone."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/emberandoak-1/800/600", AltText: "Café interior with wood-fired oven"},
			{URL: "https://picsum.photos/seed/emberandoak-2/800/600", AltText: "Brunch plate"},
			{URL: "https://picsum.photos/seed/emberandoak-3/800/600", AltText: "Coffee being poured"},
			{URL: "https://picsum.photos/seed/emberandoak-4/800/600", AltText: "Outdoor seating area"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do you take reservations?", Answer: "We take bookings for groups of 6+; smaller tables are walk-in only."},
			{Question: "Do you cater for dietary requirements?", Answer: "Yes — most of our menu has vegetarian and gluten-free options, just ask."},
		},
		staff: []domain.StaffMember{
			{Name: "Owen Reid", Role: "Head Chef & Owner", PhotoURL: "https://picsum.photos/seed/emberandoak-owen/400/400", Bio: "Ex-restaurant chef who opened Ember & Oak to slow things down."},
		},
		hours:   weekdayHours("07:30", "16:00", "08:00", "17:00"),
		social:  []domain.SocialLink{{Platform: domain.SocialInstagram, URL: "https://instagram.com/emberandoakcafe"}, {Platform: domain.SocialFacebook, URL: "https://facebook.com/emberandoakcafe"}},
		reviews: domain.SiteReviews{Rating: "4.7", ReviewCount: 203, ReviewURL: "https://google.com/search?q=ember+and+oak+cafe+reviews"},
	},
	{
		slug:         "demo-market",
		templateID:   "market",
		palette:      "onyx",
		formType:     domain.FormTypeContact,
		businessName: "Market Row Boutique",
		tagline:      "Independent fashion and homeware, curated in-store and online.",
		about:        "Market Row Boutique stocks a rotating edit of independent clothing and homeware brands, chosen for quality over trend. Pop into our storefront or browse online — every piece is picked by the same two people who run the shop.",
		ctaText:      "Shop the collection",
		contact:      domain.SiteContact{Phone: "0131 496 0142", Email: "shop@marketrowboutique.example", Address: "19 Cobble Lane, Edinburgh", Location: "Edinburgh, UK", MapURL: "https://maps.google.com/?q=Edinburgh"},
		services: []domain.Service{
			{Label: "In-store styling", Description: "Free 30-minute styling session, no obligation.", PriceText: "free"},
			{Label: "Gift wrapping", Description: "Complimentary gift wrapping on every order.", PriceText: "free"},
			{Label: "Local delivery", Description: "Same-day delivery within Edinburgh.", PriceText: "£4.50"},
		},
		testimonials: []domain.Testimonial{
			{AuthorName: "Isla F.", AuthorRole: "Customer", Quote: "Always find something I wouldn't see anywhere else on the high street."},
			{AuthorName: "Ben O.", AuthorRole: "Customer", Quote: "The styling session helped me actually finish my wardrobe, not just add to it."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/marketrow-1/800/600", AltText: "Boutique storefront"},
			{URL: "https://picsum.photos/seed/marketrow-2/800/600", AltText: "Clothing rail display"},
			{URL: "https://picsum.photos/seed/marketrow-3/800/600", AltText: "Homeware shelf"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do you ship outside the UK?", Answer: "Yes, we ship across Europe — shipping costs are calculated at checkout."},
			{Question: "What's your returns policy?", Answer: "Unworn items can be returned within 14 days for a full refund."},
		},
		staff: []domain.StaffMember{
			{Name: "Maya Cross", Role: "Co-owner & Buyer", PhotoURL: "https://picsum.photos/seed/marketrow-maya/400/400", Bio: "Sources every brand in the shop personally."},
		},
		hours:   weekdayHours("10:00", "18:00", "10:00", "17:00"),
		social:  []domain.SocialLink{{Platform: domain.SocialInstagram, URL: "https://instagram.com/marketrowboutique"}},
		reviews: domain.SiteReviews{Rating: "4.8", ReviewCount: 56, ReviewURL: "https://google.com/search?q=market+row+boutique+reviews"},
	},
	{
		slug:         "demo-surge",
		templateID:   "surge",
		palette:      "volt",
		formType:     domain.FormTypeBooking,
		businessName: "Surge Fitness Collective",
		tagline:      "High-energy group training and coaching that actually fits your week.",
		about:        "Surge Fitness Collective runs small-group strength and conditioning classes plus 1:1 coaching, built around real schedules rather than gym-bro ideals. New members get a free trial session before committing to a plan.",
		ctaText:      "Claim your free session",
		contact:      domain.SiteContact{Phone: "0151 496 0142", Email: "coach@surgefitness.example", Address: "3 Ironworks Yard, Liverpool", Location: "Liverpool, UK", MapURL: "https://maps.google.com/?q=Liverpool"},
		services: []domain.Service{
			{Label: "Small-group training", Description: "Max 8 people per class, strength and conditioning.", PriceText: "from £15/class"},
			{Label: "1:1 coaching", Description: "Personalised programming with weekly check-ins.", PriceText: "from £45/session"},
			{Label: "Monthly membership", Description: "Unlimited group classes.", PriceText: "£99/month"},
		},
		certifications: []domain.Certification{{Label: "REPs Level 3 coaches"}, {Label: "First Aid certified"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Jamie L.", AuthorRole: "Member since 2023", Quote: "First gym I've actually stuck with — the coaches remember your name and your goals."},
			{AuthorName: "Ade O.", AuthorRole: "1:1 client", Quote: "Hit a deadlift PB within 3 months of starting 1:1 coaching here."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/surgefitness-1/800/600", AltText: "Group training session"},
			{URL: "https://picsum.photos/seed/surgefitness-2/800/600", AltText: "Strength training equipment"},
			{URL: "https://picsum.photos/seed/surgefitness-3/800/600", AltText: "Coach spotting a lift"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do I need experience to join a class?", Answer: "No — every class is coached and scaled to your level, beginners welcome."},
			{Question: "Is the free trial really free?", Answer: "Yes, one free group session with no obligation to sign up."},
		},
		staff: []domain.StaffMember{
			{Name: "Leon Marsh", Role: "Head Coach", PhotoURL: "https://picsum.photos/seed/surgefitness-leon/400/400", Bio: "REPs Level 3 coach specialising in strength and conditioning."},
			{Name: "Priya Anand", Role: "Coach", PhotoURL: "https://picsum.photos/seed/surgefitness-priya/400/400", Bio: "Runs our small-group conditioning classes."},
		},
		hours:   weekdayHours("06:00", "21:00", "08:00", "14:00"),
		social:  []domain.SocialLink{{Platform: domain.SocialInstagram, URL: "https://instagram.com/surgefitnesscollective"}, {Platform: domain.SocialYouTube, URL: "https://youtube.com/@surgefitnesscollective"}},
		reviews: domain.SiteReviews{Rating: "4.9", ReviewCount: 118, ReviewURL: "https://google.com/search?q=surge+fitness+collective+reviews"},
	},
}

// SeedDemoSites ensures the built-in showcase demo sites exist and are
// fully populated — one per template, owned by ownerID — so the /templates
// gallery has a live example that shows off every content section the
// builder supports, before any real customer opts into a showcase (#39).
// Idempotent and safe to call on every boot: a demo site is created if
// missing, and its content sections are always re-synced (so a code change
// to the seed data takes effect on the next deploy) without touching the
// underlying site row or slug.
func (s *Sites) SeedDemoSites(ctx context.Context, ownerID uuid.UUID) error {
	for _, d := range demoSites {
		siteID, err := s.upsertDemoSite(ctx, ownerID, d)
		if err != nil {
			return fmt.Errorf("seed demo site %s: %w", d.slug, err)
		}
		slog.Info("demo site seeded", "slug", d.slug, "template", d.templateID, "site_id", siteID)
	}
	return nil
}

func (s *Sites) upsertDemoSite(ctx context.Context, ownerID uuid.UUID, d demoSite) (int, error) {
	existing, err := postgres.GetSiteBySlug(ctx, s.store.DB(), d.slug)
	if err != nil {
		return 0, fmt.Errorf("check site: %w", err)
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	siteID := 0
	if existing == nil {
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
		siteID, err = postgres.CreateSite(ctx, tx, site)
		if err != nil {
			return 0, fmt.Errorf("create site: %w", err)
		}
		if err := postgres.CreateDemoSiteBilling(ctx, tx, siteID); err != nil {
			return 0, fmt.Errorf("create billing: %w", err)
		}
		if err := postgres.UpsertSiteAnalyticsSettings(ctx, tx, &domain.SiteAnalyticsSettings{SiteID: siteID, AnalyticsFrequency: "off"}); err != nil {
			return 0, fmt.Errorf("save analytics settings: %w", err)
		}
	} else {
		siteID = existing.ID
	}

	if err := postgres.UpdateSiteFormType(ctx, tx, siteID, d.formType); err != nil {
		return 0, fmt.Errorf("save form type: %w", err)
	}
	d.contact.SiteID = siteID
	if err := postgres.UpsertSiteContact(ctx, tx, &d.contact); err != nil {
		return 0, fmt.Errorf("save contact: %w", err)
	}
	if err := postgres.ReplaceSiteServices(ctx, tx, siteID, d.services); err != nil {
		return 0, fmt.Errorf("save services: %w", err)
	}
	if err := postgres.ReplaceSiteCertifications(ctx, tx, siteID, d.certifications); err != nil {
		return 0, fmt.Errorf("save certifications: %w", err)
	}
	if err := postgres.ReplaceSiteTestimonials(ctx, tx, siteID, d.testimonials); err != nil {
		return 0, fmt.Errorf("save testimonials: %w", err)
	}
	if err := postgres.ReplaceSiteGalleryImages(ctx, tx, siteID, d.gallery); err != nil {
		return 0, fmt.Errorf("save gallery: %w", err)
	}
	if err := postgres.ReplaceSiteFAQItems(ctx, tx, siteID, d.faqItems); err != nil {
		return 0, fmt.Errorf("save FAQ items: %w", err)
	}
	if err := postgres.ReplaceSiteStaffMembers(ctx, tx, siteID, d.staff); err != nil {
		return 0, fmt.Errorf("save staff members: %w", err)
	}
	if err := postgres.ReplaceSiteBusinessHours(ctx, tx, siteID, d.hours); err != nil {
		return 0, fmt.Errorf("save business hours: %w", err)
	}
	if err := postgres.ReplaceSiteServiceAreas(ctx, tx, siteID, d.serviceAreas); err != nil {
		return 0, fmt.Errorf("save service areas: %w", err)
	}
	if err := postgres.ReplaceSiteSocialLinks(ctx, tx, siteID, d.social); err != nil {
		return 0, fmt.Errorf("save social links: %w", err)
	}
	d.reviews.SiteID = siteID
	if err := postgres.UpsertSiteReviews(ctx, tx, &d.reviews); err != nil {
		return 0, fmt.Errorf("save reviews: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return siteID, nil
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
