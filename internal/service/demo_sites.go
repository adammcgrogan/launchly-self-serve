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
// testimonials, gallery, FAQ, staff, hours, reviews, socials). Businesses
// are fictional but placed in real Northern Ireland towns, one per site.
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
		about:        "BrightPath has been fixing, building, and improving homes across Belfast for over a decade. From leaky taps to full room refreshes, our small team of tradespeople shows up on time, communicates clearly, and leaves your home cleaner than we found it. We started out doing odd jobs for neighbours in East Belfast and grew by word of mouth alone — we've never needed to cold-call or run adverts. Every job, big or small, gets the same care: a clear quote up front, no surprise extras, and a tidy finish.",
		ctaText:      "Get a free quote",
		contact:      domain.SiteContact{Phone: "028 9032 4567", Email: "hello@brightpathhandyman.example", Address: "14 Botanic Avenue, Belfast, BT7 1JQ", Location: "Belfast, Northern Ireland", MapURL: "https://maps.google.com/?q=Botanic+Avenue+Belfast"},
		services: []domain.Service{
			{Label: "General repairs", Description: "Odd jobs, snagging lists, and small fixes around the home.", PriceText: "from £45"},
			{Label: "Room refresh", Description: "Painting, flooring, and fixtures for a single room.", PriceText: "from £350"},
			{Label: "Flat-pack & fitting", Description: "Furniture assembly and shelving/curtain fitting.", PriceText: "from £30"},
			{Label: "Kitchen & bathroom snagging", Description: "Final-fix jobs after a renovation — sealant, trims, and fittings.", PriceText: "from £120"},
			{Label: "Gutter & fascia clearing", Description: "Seasonal gutter clearing and minor fascia repairs.", PriceText: "from £60"},
		},
		certifications: []domain.Certification{{Label: "Fully insured"}, {Label: "Access NI checked"}, {Label: "Belfast City Council registered trader"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Rachel M.", AuthorRole: "Homeowner, Stranmillis", Quote: "Turned up exactly on time and fixed three jobs in one visit. Couldn't be easier."},
			{AuthorName: "Tom D.", AuthorRole: "Landlord, Belfast", Quote: "My go-to for tenant call-outs across three properties — quick, tidy, and fair pricing."},
			{AuthorName: "Siobhan K.", AuthorRole: "Homeowner, Ormeau", Quote: "Repainted our hallway and fixed a sticking door in the same afternoon. Highly recommend."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/brightpath-1/800/600", AltText: "Freshly painted living room"},
			{URL: "https://picsum.photos/seed/brightpath-2/800/600", AltText: "Handyman fitting a shelf"},
			{URL: "https://picsum.photos/seed/brightpath-3/800/600", AltText: "Toolbox and equipment"},
			{URL: "https://picsum.photos/seed/brightpath-4/800/600", AltText: "Newly fitted kitchen shelving"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do you offer free quotes?", Answer: "Yes — send us a few photos and we'll give you a ballpark price before booking."},
			{Question: "What areas do you cover?", Answer: "Belfast and the surrounding areas, usually within a 15-mile radius."},
			{Question: "How quickly can you fit in a job?", Answer: "Most small jobs are booked within a week; call ahead for anything urgent."},
			{Question: "Do you provide your own materials?", Answer: "For small jobs, yes. For larger projects we're happy to work with materials you've already bought."},
		},
		staff: []domain.StaffMember{
			{Name: "Dave Ellison", Role: "Founder & Handyman", PhotoURL: "https://picsum.photos/seed/brightpath-dave/400/400", Bio: "10+ years fixing homes across Belfast, from Stranmillis to the Cathedral Quarter."},
			{Name: "Connor Mallon", Role: "Handyman", PhotoURL: "https://picsum.photos/seed/brightpath-connor/400/400", Bio: "Joined BrightPath in 2022, specialising in kitchen and bathroom snagging."},
		},
		hours:        weekdayHours("08:00", "17:30", "09:00", "13:00"),
		serviceAreas: []domain.ServiceArea{{Area: "Belfast"}, {Area: "Lisburn"}, {Area: "Newtownabbey"}, {Area: "Holywood"}},
		social:       []domain.SocialLink{{Platform: domain.SocialFacebook, URL: "https://facebook.com/brightpathhandyman"}, {Platform: domain.SocialInstagram, URL: "https://instagram.com/brightpathhandyman"}},
		reviews:      domain.SiteReviews{Rating: "4.9", ReviewCount: 62, ReviewURL: "https://google.com/search?q=brightpath+handyman+belfast+reviews"},
	},
	{
		slug:         "demo-foundry",
		templateID:   "foundry",
		palette:      "charcoal",
		formType:     domain.FormTypeContact,
		businessName: "Ironclad Plumbing & Heating",
		tagline:      "Emergency call-outs, boiler installs, and heating repairs across Derry~Londonderry.",
		about:        "Ironclad is a Gas Safe registered plumbing and heating team covering emergency repairs, boiler servicing, and full central heating installs across Derry~Londonderry and the wider North West. We work with homeowners and landlords alike, with same-day call-outs for burst pipes and no-heat emergencies. Founded by two apprentices-turned-engineers who trained together on the Waterside, we've built the business on turning up when we say we will and never leaving a job half-finished. Every engineer on the team carries their own Gas Safe ID — ask to see it.",
		ctaText:      "Book an engineer",
		contact:      domain.SiteContact{Phone: "028 7134 8890", Email: "jobs@ironcladplumbing.example", Address: "8 Strand Road, Derry~Londonderry, BT48 7AE", Location: "Derry~Londonderry, Northern Ireland", MapURL: "https://maps.google.com/?q=Strand+Road+Derry"},
		services: []domain.Service{
			{Label: "Emergency call-out", Description: "Burst pipes, leaks, and no-heat emergencies, same day.", PriceText: "from £80"},
			{Label: "Boiler service", Description: "Annual safety check and service for gas boilers.", PriceText: "£90"},
			{Label: "Full heating install", Description: "New central heating systems, quoted on survey.", PriceText: "from £2,800"},
			{Label: "Bathroom plumbing", Description: "New bathroom pipework, fittings, and installation.", PriceText: "from £600"},
			{Label: "Landlord safety certificates", Description: "Gas safety certificates for rental properties.", PriceText: "£65"},
		},
		certifications: []domain.Certification{{Label: "Gas Safe registered"}, {Label: "Worcester Bosch accredited"}, {Label: "OFTEC registered"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Priya S.", AuthorRole: "Homeowner, Waterside", Quote: "Boiler went out on a Sunday and they had an engineer out within two hours."},
			{AuthorName: "Mark H.", AuthorRole: "Property Manager, Derry", Quote: "We use Ironclad across all our rental properties — always professional and Gas Safe certs sorted same day."},
			{AuthorName: "Aoife C.", AuthorRole: "Homeowner, Culmore", Quote: "Full heating system replaced in two days with barely any disruption. Great value too."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/ironclad-1/800/600", AltText: "Engineer servicing a boiler"},
			{URL: "https://picsum.photos/seed/ironclad-2/800/600", AltText: "Central heating pipework"},
			{URL: "https://picsum.photos/seed/ironclad-3/800/600", AltText: "Ironclad van on site"},
			{URL: "https://picsum.photos/seed/ironclad-4/800/600", AltText: "Newly installed radiator"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Are you available for emergencies?", Answer: "Yes, we run a 24/7 emergency call-out line for burst pipes and no-heat situations."},
			{Question: "Do you offer fixed-price quotes?", Answer: "For installs and services, yes — we survey first and confirm a fixed price before starting."},
			{Question: "Do you cover landlord gas safety checks?", Answer: "Yes, we issue landlord gas safety certificates and can set up an annual reminder."},
			{Question: "How far do you travel?", Answer: "We cover Derry~Londonderry and the wider North West, including Limavady and Strabane."},
		},
		staff: []domain.StaffMember{
			{Name: "Karl Iverson", Role: "Lead Gas Engineer", PhotoURL: "https://picsum.photos/seed/ironclad-karl/400/400", Bio: "Gas Safe registered for 15 years, specialising in combi boiler installs."},
			{Name: "Josh Fenwick", Role: "Plumbing Engineer", PhotoURL: "https://picsum.photos/seed/ironclad-josh/400/400", Bio: "Handles emergency call-outs and bathroom installs."},
			{Name: "Ryan Mullan", Role: "Apprentice Engineer", PhotoURL: "https://picsum.photos/seed/ironclad-ryan/400/400", Bio: "Third-year apprentice training toward full Gas Safe registration."},
		},
		hours:        weekdayHours("07:00", "18:00", "08:00", "14:00"),
		serviceAreas: []domain.ServiceArea{{Area: "Derry~Londonderry"}, {Area: "Limavady"}, {Area: "Strabane"}, {Area: "Eglinton"}},
		social:       []domain.SocialLink{{Platform: domain.SocialFacebook, URL: "https://facebook.com/ironcladplumbing"}},
		reviews:      domain.SiteReviews{Rating: "4.8", ReviewCount: 134, ReviewURL: "https://google.com/search?q=ironclad+plumbing+derry+reviews"},
	},
	{
		slug:         "demo-meridian",
		templateID:   "meridian",
		palette:      "ivory",
		formType:     domain.FormTypeContact,
		businessName: "Thornfield Legal Associates",
		tagline:      "Considered legal advice for individuals and small businesses in Lisburn.",
		about:        "Thornfield Legal Associates provides clear, straightforward legal advice on conveyancing, wills and probate, and small business contracts. We believe good advice should be plain-spoken, not dressed up in jargon — so that's how we practise. Based on Bow Street in the heart of Lisburn, we act for clients across County Antrim and County Down, and increasingly further afield now that most of our work can be done remotely. We keep a deliberately small caseload so every client gets a solicitor who actually knows their file.",
		ctaText:      "Book a consultation",
		contact:      domain.SiteContact{Phone: "028 9266 3312", Email: "enquiries@thornfieldlegal.example", Address: "22 Bow Street, Lisburn, BT28 1BN", Location: "Lisburn, Northern Ireland", MapURL: "https://maps.google.com/?q=Bow+Street+Lisburn"},
		services: []domain.Service{
			{Label: "Residential conveyancing", Description: "Buying or selling a home, start to completion.", PriceText: "from £750"},
			{Label: "Wills & probate", Description: "Straightforward wills and probate administration.", PriceText: "from £250"},
			{Label: "Small business contracts", Description: "Terms of service, supplier agreements, and NDAs.", PriceText: "from £400"},
			{Label: "Power of attorney", Description: "Enduring power of attorney drafting and registration.", PriceText: "from £300"},
		},
		certifications: []domain.Certification{{Label: "Law Society of Northern Ireland regulated"}, {Label: "Lexcel accredited"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Alison P.", AuthorRole: "First-time buyer, Lisburn", Quote: "Explained every step in plain English — made a stressful process feel manageable."},
			{AuthorName: "Grant & Co", AuthorRole: "Small business client", Quote: "Sorted our supplier contracts quickly and at a fraction of the cost we expected."},
			{AuthorName: "Eamon B.", AuthorRole: "Probate client", Quote: "Handled a difficult probate case for our family with real patience and care."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/thornfield-1/800/600", AltText: "Meeting room at Thornfield Legal"},
			{URL: "https://picsum.photos/seed/thornfield-2/800/600", AltText: "Reception desk"},
			{URL: "https://picsum.photos/seed/thornfield-3/800/600", AltText: "Bow Street office exterior"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do you offer a free initial consultation?", Answer: "Yes, the first 20 minutes of any new matter is free of charge."},
			{Question: "Do you handle cases outside Lisburn?", Answer: "Most of our work is done remotely, so we act for clients across Northern Ireland."},
			{Question: "How long does conveyancing usually take?", Answer: "Typically 8-12 weeks from offer to completion, depending on the chain."},
		},
		staff: []domain.StaffMember{
			{Name: "Eleanor Thornfield", Role: "Founding Solicitor", PhotoURL: "https://picsum.photos/seed/thornfield-eleanor/400/400", Bio: "20 years' experience in residential property and private client law."},
			{Name: "Conor McAllister", Role: "Associate Solicitor", PhotoURL: "https://picsum.photos/seed/thornfield-conor/400/400", Bio: "Handles small business contracts and commercial leases."},
		},
		hours:   weekdayHours("09:00", "17:30", "", ""),
		social:  []domain.SocialLink{{Platform: domain.SocialLinkedIn, URL: "https://linkedin.com/company/thornfield-legal"}},
		reviews: domain.SiteReviews{Rating: "4.9", ReviewCount: 41, ReviewURL: "https://google.com/search?q=thornfield+legal+lisburn+reviews"},
	},
	{
		slug:         "demo-bloom",
		templateID:   "bloom",
		palette:      "blush",
		formType:     domain.FormTypeBooking,
		businessName: "Bloom & Co Hair Studio",
		tagline:      "A calm, considered studio for cut, colour, and care in Bangor.",
		about:        "Bloom & Co is an independent hair studio built around unhurried appointments and honest advice. Whether you're after a precision cut or a full colour transformation, our stylists take the time to get it right — no rushing, no upselling. We opened on Bangor's Main Street in 2019 with two chairs and a lot of ambition, and now run a team of four stylists in a bright, plant-filled studio two minutes from the seafront. Come for the cut, stay for the coffee.",
		ctaText:      "Book your appointment",
		contact:      domain.SiteContact{Phone: "028 9127 5540", Email: "book@bloomandco.example", Address: "5 Main Street, Bangor, BT20 5AG", Location: "Bangor, Northern Ireland", MapURL: "https://maps.google.com/?q=Main+Street+Bangor"},
		services: []domain.Service{
			{Label: "Cut & finish", Description: "Consultation, wash, cut, and blow-dry.", PriceText: "from £42"},
			{Label: "Full colour", Description: "Root-to-tip colour with gloss finish.", PriceText: "from £95"},
			{Label: "Balayage", Description: "Hand-painted highlights for a natural, sun-kissed look.", PriceText: "from £130"},
			{Label: "Bridal hair", Description: "Trial and on-the-day styling for your wedding party.", PriceText: "from £180"},
		},
		certifications: []domain.Certification{{Label: "L'Oréal Colour Specialist"}, {Label: "Wella Master Colour Expert"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Chloe B.", AuthorRole: "Regular client", Quote: "Best balayage I've ever had, and the studio itself is so relaxing."},
			{AuthorName: "Nisha R.", AuthorRole: "New client", Quote: "Booked online in seconds and my stylist really listened to what I wanted."},
			{AuthorName: "Grace M.", AuthorRole: "Bride, 2025", Quote: "Did my bridal trial and the big day itself — hair held up perfectly all night."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/bloomandco-1/800/600", AltText: "Hair studio interior"},
			{URL: "https://picsum.photos/seed/bloomandco-2/800/600", AltText: "Balayage colour result"},
			{URL: "https://picsum.photos/seed/bloomandco-3/800/600", AltText: "Styling station"},
			{URL: "https://picsum.photos/seed/bloomandco-4/800/600", AltText: "Bridal hair styling"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do I need a patch test before colour?", Answer: "Yes, for any first-time colour appointment we require a patch test 48 hours beforehand."},
			{Question: "Can I book online?", Answer: "Yes — use the booking button above to pick a stylist and time that suits you."},
			{Question: "Do you do bridal trials?", Answer: "Yes, we recommend booking a trial 6-8 weeks before your wedding date."},
		},
		staff: []domain.StaffMember{
			{Name: "Amara Bloom", Role: "Founder & Colourist", PhotoURL: "https://picsum.photos/seed/bloomandco-amara/400/400", Bio: "Specialises in balayage and colour correction."},
			{Name: "Freya Lund", Role: "Senior Stylist", PhotoURL: "https://picsum.photos/seed/bloomandco-freya/400/400", Bio: "Cutting specialist with a focus on low-maintenance styles."},
			{Name: "Orla Kane", Role: "Bridal Stylist", PhotoURL: "https://picsum.photos/seed/bloomandco-orla/400/400", Bio: "Runs our bridal and occasion styling bookings."},
		},
		hours:   weekdayHours("09:30", "18:00", "09:00", "16:00"),
		social:  []domain.SocialLink{{Platform: domain.SocialInstagram, URL: "https://instagram.com/bloomandcostudio"}, {Platform: domain.SocialTikTok, URL: "https://tiktok.com/@bloomandcostudio"}},
		reviews: domain.SiteReviews{Rating: "5.0", ReviewCount: 87, ReviewURL: "https://google.com/search?q=bloom+and+co+hair+studio+bangor+reviews"},
	},
	{
		slug:         "demo-ember",
		templateID:   "ember",
		palette:      "terracotta",
		formType:     domain.FormTypeContact,
		businessName: "Ember & Oak Café",
		tagline:      "Wood-fired brunch and slow coffee in the heart of Newry.",
		about:        "Ember & Oak is a neighbourhood café serving wood-fired brunch, seasonal small plates, and coffee roasted five minutes down the road. We're open early for the commute crowd and stay cosy long after for something slower. Tucked just off Hill Street in Newry's city centre, we built the whole menu around a single wood-fired oven — everything from the sourdough to the shakshuka passes through it. Local suppliers get a mention on the specials board every week.",
		ctaText:      "View our menu",
		contact:      domain.SiteContact{Phone: "028 3025 6178", Email: "hello@emberandoak.example", Address: "31 Hill Street, Newry, BT34 1AR", Location: "Newry, Northern Ireland", MapURL: "https://maps.google.com/?q=Hill+Street+Newry"},
		services: []domain.Service{
			{Label: "Wood-fired brunch", Description: "Served all day, weekends till 3pm.", PriceText: "from £9"},
			{Label: "Seasonal small plates", Description: "Changing weekly with local produce.", PriceText: "from £7"},
			{Label: "Private bookings", Description: "Our back room seats up to 20 for events.", PriceText: "on request"},
			{Label: "Coffee subscription", Description: "Fortnightly bag of our house roast, collect in-store.", PriceText: "£12/fortnight"},
		},
		testimonials: []domain.Testimonial{
			{AuthorName: "Callum W.", AuthorRole: "Regular", Quote: "Best flat white in the city and the sourdough is unreal."},
			{AuthorName: "Sana K.", AuthorRole: "First visit", Quote: "Cosy, unpretentious, and the wood-fired eggs were worth the trip alone."},
			{AuthorName: "Declan F.", AuthorRole: "Local business owner", Quote: "Booked the back room for a team lunch — food and service were spot on."},
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
			{Question: "Can I buy your coffee beans?", Answer: "Yes, bags are available at the counter or via our fortnightly subscription."},
		},
		staff: []domain.StaffMember{
			{Name: "Owen Reid", Role: "Head Chef & Owner", PhotoURL: "https://picsum.photos/seed/emberandoak-owen/400/400", Bio: "Ex-restaurant chef who opened Ember & Oak to slow things down."},
			{Name: "Niamh Cassidy", Role: "Head Barista", PhotoURL: "https://picsum.photos/seed/emberandoak-niamh/400/400", Bio: "Runs the coffee program and roast selection."},
		},
		hours:   weekdayHours("07:30", "16:00", "08:00", "17:00"),
		social:  []domain.SocialLink{{Platform: domain.SocialInstagram, URL: "https://instagram.com/emberandoakcafe"}, {Platform: domain.SocialFacebook, URL: "https://facebook.com/emberandoakcafe"}},
		reviews: domain.SiteReviews{Rating: "4.7", ReviewCount: 203, ReviewURL: "https://google.com/search?q=ember+and+oak+cafe+newry+reviews"},
	},
	{
		slug:         "demo-market",
		templateID:   "market",
		palette:      "onyx",
		formType:     domain.FormTypeContact,
		businessName: "Market Row Boutique",
		tagline:      "Independent fashion and homeware, curated in-store and online in Ballymena.",
		about:        "Market Row Boutique stocks a rotating edit of independent clothing and homeware brands, chosen for quality over trend. Pop into our storefront or browse online — every piece is picked by the same two people who run the shop. We've traded on Ballymena's Wellington Street since 2017, and built a loyal following by refusing to stock anything we wouldn't wear or use ourselves. Around a third of our range comes from small Northern Irish makers and designers.",
		ctaText:      "Shop the collection",
		contact:      domain.SiteContact{Phone: "028 2565 1190", Email: "shop@marketrowboutique.example", Address: "19 Wellington Street, Ballymena, BT43 6EJ", Location: "Ballymena, Northern Ireland", MapURL: "https://maps.google.com/?q=Wellington+Street+Ballymena"},
		services: []domain.Service{
			{Label: "In-store styling", Description: "Free 30-minute styling session, no obligation.", PriceText: "free"},
			{Label: "Gift wrapping", Description: "Complimentary gift wrapping on every order.", PriceText: "free"},
			{Label: "Local delivery", Description: "Same-day delivery within Ballymena.", PriceText: "£4.50"},
			{Label: "Personal shopping", Description: "One-to-one wardrobe session by appointment.", PriceText: "£25"},
		},
		testimonials: []domain.Testimonial{
			{AuthorName: "Isla F.", AuthorRole: "Customer", Quote: "Always find something I wouldn't see anywhere else on the high street."},
			{AuthorName: "Ben O.", AuthorRole: "Customer", Quote: "The styling session helped me actually finish my wardrobe, not just add to it."},
			{AuthorName: "Maeve T.", AuthorRole: "Customer", Quote: "Love that so much of the range is made locally — you can tell they care about it."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/marketrow-1/800/600", AltText: "Boutique storefront"},
			{URL: "https://picsum.photos/seed/marketrow-2/800/600", AltText: "Clothing rail display"},
			{URL: "https://picsum.photos/seed/marketrow-3/800/600", AltText: "Homeware shelf"},
			{URL: "https://picsum.photos/seed/marketrow-4/800/600", AltText: "Gift wrapping counter"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do you ship outside Northern Ireland?", Answer: "Yes, we ship across the UK and Ireland — shipping costs are calculated at checkout."},
			{Question: "What's your returns policy?", Answer: "Unworn items can be returned within 14 days for a full refund."},
			{Question: "Do you stock local designers?", Answer: "Yes, around a third of our range is made by Northern Irish designers and makers."},
		},
		staff: []domain.StaffMember{
			{Name: "Maya Cross", Role: "Co-owner & Buyer", PhotoURL: "https://picsum.photos/seed/marketrow-maya/400/400", Bio: "Sources every brand in the shop personally."},
			{Name: "Ciara Boyd", Role: "Co-owner & Stylist", PhotoURL: "https://picsum.photos/seed/marketrow-ciara/400/400", Bio: "Runs the in-store styling and personal shopping sessions."},
		},
		hours:   weekdayHours("10:00", "18:00", "10:00", "17:00"),
		social:  []domain.SocialLink{{Platform: domain.SocialInstagram, URL: "https://instagram.com/marketrowboutique"}},
		reviews: domain.SiteReviews{Rating: "4.8", ReviewCount: 56, ReviewURL: "https://google.com/search?q=market+row+boutique+ballymena+reviews"},
	},
	{
		slug:         "demo-surge",
		templateID:   "surge",
		palette:      "volt",
		formType:     domain.FormTypeBooking,
		businessName: "Surge Fitness Collective",
		tagline:      "High-energy group training and coaching that actually fits your week, in Craigavon.",
		about:        "Surge Fitness Collective runs small-group strength and conditioning classes plus 1:1 coaching, built around real schedules rather than gym-bro ideals. New members get a free trial session before committing to a plan. We train out of a converted warehouse unit on the edge of Craigavon, kitted out with proper strength equipment rather than rows of cardio machines. Most of our members started with zero gym experience — the coaching is built for that.",
		ctaText:      "Claim your free session",
		contact:      domain.SiteContact{Phone: "028 3833 7724", Email: "coach@surgefitness.example", Address: "3 Ironworks Business Park, Craigavon, BT64 1AA", Location: "Craigavon, Northern Ireland", MapURL: "https://maps.google.com/?q=Craigavon"},
		services: []domain.Service{
			{Label: "Small-group training", Description: "Max 8 people per class, strength and conditioning.", PriceText: "from £15/class"},
			{Label: "1:1 coaching", Description: "Personalised programming with weekly check-ins.", PriceText: "from £45/session"},
			{Label: "Monthly membership", Description: "Unlimited group classes.", PriceText: "£99/month"},
			{Label: "Nutrition coaching", Description: "Monthly check-ins and meal planning support.", PriceText: "from £35/month"},
		},
		certifications: []domain.Certification{{Label: "REPs Level 3 coaches"}, {Label: "First Aid certified"}, {Label: "Precision Nutrition certified"}},
		testimonials: []domain.Testimonial{
			{AuthorName: "Jamie L.", AuthorRole: "Member since 2023", Quote: "First gym I've actually stuck with — the coaches remember your name and your goals."},
			{AuthorName: "Ade O.", AuthorRole: "1:1 client", Quote: "Hit a deadlift PB within 3 months of starting 1:1 coaching here."},
			{AuthorName: "Emma R.", AuthorRole: "Nutrition client", Quote: "The nutrition coaching finally made healthy eating feel sustainable, not restrictive."},
		},
		gallery: []domain.GalleryImage{
			{URL: "https://picsum.photos/seed/surgefitness-1/800/600", AltText: "Group training session"},
			{URL: "https://picsum.photos/seed/surgefitness-2/800/600", AltText: "Strength training equipment"},
			{URL: "https://picsum.photos/seed/surgefitness-3/800/600", AltText: "Coach spotting a lift"},
			{URL: "https://picsum.photos/seed/surgefitness-4/800/600", AltText: "Warehouse gym floor"},
		},
		faqItems: []domain.FAQItem{
			{Question: "Do I need experience to join a class?", Answer: "No — every class is coached and scaled to your level, beginners welcome."},
			{Question: "Is the free trial really free?", Answer: "Yes, one free group session with no obligation to sign up."},
			{Question: "Do you offer nutrition coaching on its own?", Answer: "Yes, it can be added to any membership or booked standalone."},
		},
		staff: []domain.StaffMember{
			{Name: "Leon Marsh", Role: "Head Coach", PhotoURL: "https://picsum.photos/seed/surgefitness-leon/400/400", Bio: "REPs Level 3 coach specialising in strength and conditioning."},
			{Name: "Priya Anand", Role: "Coach", PhotoURL: "https://picsum.photos/seed/surgefitness-priya/400/400", Bio: "Runs our small-group conditioning classes."},
			{Name: "Sam Devlin", Role: "Nutrition Coach", PhotoURL: "https://picsum.photos/seed/surgefitness-sam/400/400", Bio: "Precision Nutrition certified, runs our monthly nutrition check-ins."},
		},
		hours:   weekdayHours("06:00", "21:00", "08:00", "14:00"),
		social:  []domain.SocialLink{{Platform: domain.SocialInstagram, URL: "https://instagram.com/surgefitnesscollective"}, {Platform: domain.SocialYouTube, URL: "https://youtube.com/@surgefitnesscollective"}},
		reviews: domain.SiteReviews{Rating: "4.9", ReviewCount: 118, ReviewURL: "https://google.com/search?q=surge+fitness+collective+craigavon+reviews"},
	},
}

// SeedDemoSites ensures the built-in showcase demo sites exist — one per
// template, owned by ownerID — so the /templates gallery has a live example
// that shows off every content section the builder supports, before any
// real customer opts into a showcase (#39). Seeds once: a demo slug that
// already exists is left untouched (no content re-sync), so this is cheap
// and quiet to call on every boot. To push a seed-data change to an
// already-seeded site, delete its row (by slug) and let it reseed.
func (s *Sites) SeedDemoSites(ctx context.Context, ownerID uuid.UUID) error {
	for _, d := range demoSites {
		created, err := s.createDemoSiteIfMissing(ctx, ownerID, d)
		if err != nil {
			return fmt.Errorf("seed demo site %s: %w", d.slug, err)
		}
		if created {
			slog.Info("demo site seeded", "slug", d.slug, "template", d.templateID)
		}
	}
	return nil
}

// createDemoSiteIfMissing creates and fully populates one demo site, or
// does nothing if a site with that slug already exists. Returns whether it
// created a new site.
func (s *Sites) createDemoSiteIfMissing(ctx context.Context, ownerID uuid.UUID, d demoSite) (bool, error) {
	existing, err := postgres.GetSiteBySlug(ctx, s.store.DB(), d.slug)
	if err != nil {
		return false, fmt.Errorf("check site: %w", err)
	}
	if existing != nil {
		return false, nil
	}

	tx, err := s.store.BeginTx(ctx)
	if err != nil {
		return false, err
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
		return false, fmt.Errorf("create site: %w", err)
	}
	if err := postgres.CreateDemoSiteBilling(ctx, tx, siteID); err != nil {
		return false, fmt.Errorf("create billing: %w", err)
	}
	if err := postgres.UpsertSiteAnalyticsSettings(ctx, tx, &domain.SiteAnalyticsSettings{SiteID: siteID, AnalyticsFrequency: "off"}); err != nil {
		return false, fmt.Errorf("save analytics settings: %w", err)
	}

	if err := postgres.UpdateSiteFormType(ctx, tx, siteID, d.formType); err != nil {
		return false, fmt.Errorf("save form type: %w", err)
	}
	d.contact.SiteID = siteID
	if err := postgres.UpsertSiteContact(ctx, tx, &d.contact); err != nil {
		return false, fmt.Errorf("save contact: %w", err)
	}
	if err := postgres.ReplaceSiteServices(ctx, tx, siteID, d.services); err != nil {
		return false, fmt.Errorf("save services: %w", err)
	}
	if err := postgres.ReplaceSiteCertifications(ctx, tx, siteID, d.certifications); err != nil {
		return false, fmt.Errorf("save certifications: %w", err)
	}
	if err := postgres.ReplaceSiteTestimonials(ctx, tx, siteID, d.testimonials); err != nil {
		return false, fmt.Errorf("save testimonials: %w", err)
	}
	if err := postgres.ReplaceSiteGalleryImages(ctx, tx, siteID, d.gallery); err != nil {
		return false, fmt.Errorf("save gallery: %w", err)
	}
	if err := postgres.ReplaceSiteFAQItems(ctx, tx, siteID, d.faqItems); err != nil {
		return false, fmt.Errorf("save FAQ items: %w", err)
	}
	if err := postgres.ReplaceSiteStaffMembers(ctx, tx, siteID, d.staff); err != nil {
		return false, fmt.Errorf("save staff members: %w", err)
	}
	if err := postgres.ReplaceSiteBusinessHours(ctx, tx, siteID, d.hours); err != nil {
		return false, fmt.Errorf("save business hours: %w", err)
	}
	if err := postgres.ReplaceSiteServiceAreas(ctx, tx, siteID, d.serviceAreas); err != nil {
		return false, fmt.Errorf("save service areas: %w", err)
	}
	if err := postgres.ReplaceSiteSocialLinks(ctx, tx, siteID, d.social); err != nil {
		return false, fmt.Errorf("save social links: %w", err)
	}
	d.reviews.SiteID = siteID
	if err := postgres.UpsertSiteReviews(ctx, tx, &d.reviews); err != nil {
		return false, fmt.Errorf("save reviews: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit: %w", err)
	}
	return true, nil
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
