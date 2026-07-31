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
	EventKindDownload   EventKind = "download"
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

// PageCount is a page path with its view count.
type PageCount struct {
	Path  string
	Count int
}

// SiteStats holds aggregated analytics for a site over a period.
type SiteStats struct {
	TotalViews     int
	UniqueVisitors int
	TopReferrers   []ReferrerCount
	TopPages       []PageCount
	ViewsByDay     []DayCount
	PeriodDays     int

	CallTaps          int
	WhatsAppTaps      int
	DirectionsClicks  int
	Leads             int
	DocumentDownloads int

	// Prev* mirror the same window immediately preceding the current
	// period, for a period-over-period comparison. They're zero when the
	// caller didn't request a comparison window (see GetSiteStats).
	PrevTotalViews     int
	PrevUniqueVisitors int
	PrevLeads          int
}

// TotalConversions sums every conversion kind — the number that proves the
// site pays for itself, as opposed to raw page views.
func (s SiteStats) TotalConversions() int {
	return s.CallTaps + s.WhatsAppTaps + s.DirectionsClicks + s.Leads + s.DocumentDownloads
}

// ConversionRate is conversions as a percentage of unique visitors — the
// single number that answers "is this site working for me". Zero when
// there's no traffic to divide by.
func (s SiteStats) ConversionRate() float64 {
	if s.UniqueVisitors == 0 {
		return 0
	}
	return float64(s.TotalConversions()) * 100 / float64(s.UniqueVisitors)
}

// ViewsChangePct, UniqueVisitorsChangePct, and LeadsChangePct return the
// period-over-period percentage change for each headline stat. Only
// meaningful when the matching Prev* field is non-zero (see PrevTotalViews
// et al.) — templates guard on that field before calling these, since
// text/template comparison funcs can't handle a nil *int cleanly.
func (s SiteStats) ViewsChangePct() int {
	return (s.TotalViews - s.PrevTotalViews) * 100 / s.PrevTotalViews
}
func (s SiteStats) UniqueVisitorsChangePct() int {
	return (s.UniqueVisitors - s.PrevUniqueVisitors) * 100 / s.PrevUniqueVisitors
}
func (s SiteStats) LeadsChangePct() int {
	return (s.Leads - s.PrevLeads) * 100 / s.PrevLeads
}
