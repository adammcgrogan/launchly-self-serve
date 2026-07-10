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
}
