package domain

import (
	"time"

	"github.com/google/uuid"
)

// SiteMemberRole is the only non-owner role today — the owner themselves is
// never a site_members row, they're identified by sites.owner_user_id.
type SiteMemberRole string

const SiteMemberRoleMember SiteMemberRole = "member"

type SiteMemberStatus string

const (
	SiteMemberStatusPending  SiteMemberStatus = "pending"
	SiteMemberStatusAccepted SiteMemberStatus = "accepted"
)

// SiteMember is a teammate invited to help manage a site's content and
// leads. UserID is unset until the invited email accepts — the invite may
// be sent to an address that doesn't have a Launchly account yet.
type SiteMember struct {
	ID          int
	SiteID      int
	UserID      *uuid.UUID
	Email       string
	Role        SiteMemberRole
	Status      SiteMemberStatus
	InviteToken string
	InvitedAt   time.Time
	AcceptedAt  *time.Time
}
