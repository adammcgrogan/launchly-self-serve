package domain

import (
	"time"

	"github.com/google/uuid"
)

type SiteStatus string

const (
	SiteStatusDraft  SiteStatus = "draft"
	SiteStatusLive   SiteStatus = "live"
	SiteStatusPaused SiteStatus = "paused" // trial ended (+ grace) with no paid plan; distinct from owner-chosen draft
)

type Plan string

const (
	PlanStarter Plan = "starter"
	PlanPro     Plan = "pro"
)

type CustomDomainStatus string

const (
	CustomDomainNone    CustomDomainStatus = "none"
	CustomDomainPending CustomDomainStatus = "pending"
	CustomDomainActive  CustomDomainStatus = "active"
	CustomDomainFailed  CustomDomainStatus = "failed"
)

type PaymentStatus string

const (
	PaymentStatusTrialing  PaymentStatus = "trialing"
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusPaid      PaymentStatus = "paid"
	PaymentStatusCancelled PaymentStatus = "cancelled"
)

type SocialPlatform string

const (
	SocialFacebook  SocialPlatform = "facebook"
	SocialInstagram SocialPlatform = "instagram"
	SocialWhatsApp  SocialPlatform = "whatsapp"
	SocialTwitter   SocialPlatform = "twitter"
	SocialTikTok    SocialPlatform = "tiktok"
	SocialLinkedIn  SocialPlatform = "linkedin"
	SocialYouTube   SocialPlatform = "youtube"
)

// Site is the core identity of a business's site. Everything else that
// belongs to a site (contact info, billing, social links, etc.) lives in
// its own table/struct and is loaded alongside it as a SiteAggregate.
type Site struct {
	ID            int
	OwnerUserID   uuid.UUID
	Slug          string
	BusinessName  string
	Tagline       string
	About         string
	LogoURL       string
	CTAText       string
	TemplateID    string
	Palette       string
	HeadingFont   string
	Status        SiteStatus
	CreatedAt     time.Time
	PublishedAt   *time.Time
	UpdatedAt     time.Time
	SlugChangedAt *time.Time

	CustomDomain        string
	CustomDomainStatus  CustomDomainStatus
	CustomDomainCFID    string
	CustomDomainAddedAt *time.Time
}

// SiteContact holds a site's public contact details. 1:1 with Site.
type SiteContact struct {
	SiteID      int
	Phone       string
	Email       string
	Address     string
	Location    string // short location for hero badge, e.g. "Belfast, NI"
	MapURL      string
	MapEmbedURL string
}

// SiteBilling holds a site's plan, trial, and Stripe state. 1:1 with Site.
type SiteBilling struct {
	SiteID                   int
	Plan                     Plan
	PaymentStatus            PaymentStatus
	StripeCustomerID         string
	StripeSessionID          string
	StripeSubscriptionID     string
	PaidAt                   *time.Time
	TrialEndsAt              *time.Time
	TrialReminderSentAt      *time.Time
	TrialFinalReminderSentAt *time.Time
}

// SiteAnalyticsSettings holds a site's analytics preferences. 1:1 with Site.
type SiteAnalyticsSettings struct {
	SiteID              int
	UmamiWebsiteID      string
	AnalyticsFrequency  string // "off", "weekly", "monthly"
	AnalyticsLastSentAt *time.Time
}

// SiteNotifySettings holds a site's opt-in SMS lead alert preferences. 1:1
// with Site. SMS is a Pro perk on top of the always-on email notification.
type SiteNotifySettings struct {
	SiteID           int
	MobileNumber     string // E.164 format, e.g. "+447700900123"
	SMSAlertsEnabled bool
}

// SiteAnnouncement is a temporary banner an owner can set from the
// dashboard (e.g. "Closed for holidays until 4 Aug"), shown on every page
// until it's cleared or ExpiresAt passes. 1:1 with Site.
type SiteAnnouncement struct {
	SiteID    int
	Text      string
	ExpiresAt *time.Time
}

// Active reports whether the announcement should currently be shown.
// ExpiresAt is a calendar date (midnight UTC) and is inclusive of that
// whole day, e.g. an expiry of "4 Aug" keeps the banner up through 4 Aug.
func (a SiteAnnouncement) Active() bool {
	if a.Text == "" {
		return false
	}
	return a.ExpiresAt == nil || time.Now().Before(a.ExpiresAt.Add(24*time.Hour))
}

// SocialLink is one social platform URL for a site. Many-to-one with Site.
type SocialLink struct {
	ID       int
	SiteID   int
	Platform SocialPlatform
	URL      string
}

// Service is one line item in a site's services list.
type Service struct {
	ID        int
	SiteID    int
	Label     string
	SortOrder int
}

// Certification is one trust badge / certification shown on a site.
type Certification struct {
	ID        int
	SiteID    int
	Label     string
	SortOrder int
}

// Testimonial is one customer quote shown on a site.
type Testimonial struct {
	ID         int
	SiteID     int
	AuthorName string
	AuthorRole string
	Quote      string
	SortOrder  int
}

// GalleryImage is one photo in a site's gallery.
type GalleryImage struct {
	ID        int
	SiteID    int
	URL       string
	AltText   string
	SortOrder int
}

// BusinessHours is one opening-hours line, e.g. "Mon-Fri" / "9am-5pm".
type BusinessHours struct {
	ID        int
	SiteID    int
	Label     string
	HoursText string
	SortOrder int
}

// SiteAggregate is a fully-loaded site with all related data, as used by
// the editor, the public site renderer, and the dashboard. Assembled by
// service/sites.go from multiple repository calls.
type SiteAggregate struct {
	Site
	Contact        SiteContact
	Billing        SiteBilling
	Analytics      SiteAnalyticsSettings
	Notify         SiteNotifySettings
	Announcement   SiteAnnouncement
	SocialLinks    []SocialLink
	Services       []Service
	Certifications []Certification
	Testimonials   []Testimonial
	GalleryImages  []GalleryImage
	BusinessHours  []BusinessHours
}
