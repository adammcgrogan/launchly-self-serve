package domain

import "time"

// LeadStatus tracks an owner's follow-up progress on a lead.
type LeadStatus string

const (
	LeadStatusNew       LeadStatus = "new"
	LeadStatusContacted LeadStatus = "contacted"
	LeadStatusWon       LeadStatus = "won"
	LeadStatusLost      LeadStatus = "lost"
)

// LeadCounts summarizes a site's leads regardless of any list filter, for the
// dashboard's "leads received" stat and "N new" badge.
type LeadCounts struct {
	Total int
	New   int
}

// Lead is a contact form submission from a site visitor.
type Lead struct {
	ID            int
	SiteID        int
	Name          string
	Email         string
	Phone         string
	Message       string
	ServiceLabel  string
	PreferredTime string
	Status        LeadStatus
	CreatedAt     time.Time
}
