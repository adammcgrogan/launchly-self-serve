package domain

import "time"

// Lead is a contact form submission from a site visitor.
type Lead struct {
	ID        int
	SiteID    int
	Name      string
	Email     string
	Phone     string
	Message   string
	CreatedAt time.Time
}
