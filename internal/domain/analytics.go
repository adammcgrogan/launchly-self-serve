package domain

import "time"

// PageView is a single recorded visit to a site.
type PageView struct {
	ID          int
	SiteID      int
	Path        string
	Referrer    string
	VisitorHash string // salted hash of the visitor's IP, for approximate unique-visitor counts
	CreatedAt   time.Time
}

// EventKind identifies the type of conversion a SiteEvent records.
type EventKind string

const (
	EventKindCall       EventKind = "call"
	EventKindWhatsApp   EventKind = "whatsapp"
	EventKindDirections EventKind = "directions"
	EventKindLead       EventKind = "lead"
)

// SiteEvent is a single recorded conversion — a tel:/WhatsApp/directions
// tap or a contact-form submission — the actions that actually matter to a
// local business, as opposed to a raw page view.
type SiteEvent struct {
	ID          int
	SiteID      int
	Kind        EventKind
	VisitorHash string
	CreatedAt   time.Time
}

// ReferrerCount is a referrer hostname with its visit count.
type ReferrerCount struct {
	Referrer string
	Count    int
}

// DayCount is a single day's view count.
type DayCount struct {
	Day   time.Time
	Count int
}

// SiteStats holds aggregated analytics for a site over a period.
type SiteStats struct {
	TotalViews     int
	UniqueVisitors int
	TopReferrers   []ReferrerCount
	ViewsByDay     []DayCount
	PeriodDays     int

	CallTaps         int
	WhatsAppTaps     int
	DirectionsClicks int
	Leads            int
}

// TotalConversions sums every conversion kind — the number that proves the
// site pays for itself, as opposed to raw page views.
func (s SiteStats) TotalConversions() int {
	return s.CallTaps + s.WhatsAppTaps + s.DirectionsClicks + s.Leads
}
