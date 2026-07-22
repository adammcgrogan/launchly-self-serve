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

// Valid reports whether s is one of the recognized lead statuses.
func (s LeadStatus) Valid() bool {
	switch s {
	case LeadStatusNew, LeadStatusContacted, LeadStatusWon, LeadStatusLost:
		return true
	default:
		return false
	}
}

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
	Notes         []LeadNote
}

// LeadNote is a free-text follow-up note an owner has logged against a lead,
// oldest first.
type LeadNote struct {
	ID        int
	LeadID    int
	Body      string
	CreatedAt time.Time
}
