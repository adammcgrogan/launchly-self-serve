package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

var (
	ErrMemberAlreadyActive  = errors.New("that person is already a member of this site.")
	ErrMemberAlreadyInvited = errors.New("that person already has a pending invite.")
	ErrInviteNotFound       = errors.New("that invite link is invalid or has expired.")
	ErrInviteEmailMismatch  = errors.New("this invite was sent to a different email address — log in as that address to accept it.")
)

// Members handles inviting, accepting, listing, and removing site teammates
// (see domain.SiteMember). The owner themselves is never a row here — only
// one non-owner role exists today, so there's no role management beyond
// "is an accepted member of this site".
type Members struct {
	store   *postgres.Store
	mailer  *email.Client
	baseURL string
}

func NewMembers(store *postgres.Store, mailer *email.Client, baseURL string) *Members {
	return &Members{store: store, mailer: mailer, baseURL: baseURL}
}

// Invite sends a teammate invite email and records a pending site_members
// row. inviteeEmail may or may not already have a Launchly account — the
// accept link works either way (see Accept).
func (m *Members) Invite(ctx context.Context, site *domain.Site, inviterEmail, inviteeEmail string) (*domain.SiteMember, error) {
	inviteeEmail = strings.TrimSpace(strings.ToLower(inviteeEmail))
	if err := checkEmail("email address", inviteeEmail); err != nil {
		return nil, err
	}
	if inviteeEmail == "" {
		return nil, &ValidationError{Message: "enter an email address.", Field: "email"}
	}

	existing, err := postgres.GetSiteMemberBySiteAndEmail(ctx, m.store.DB(), site.ID, inviteeEmail)
	if err != nil {
		return nil, fmt.Errorf("check existing member: %w", err)
	}
	if existing != nil {
		if existing.Status == domain.SiteMemberStatusAccepted {
			return nil, ErrMemberAlreadyActive
		}
		return nil, ErrMemberAlreadyInvited
	}

	token, err := generateInviteToken()
	if err != nil {
		return nil, fmt.Errorf("generate invite token: %w", err)
	}
	member, err := postgres.CreateSiteMemberInvite(ctx, m.store.DB(), site.ID, inviteeEmail, token)
	if err != nil {
		return nil, fmt.Errorf("create invite: %w", err)
	}

	acceptURL := m.baseURL + "/dashboard/invites/" + token
	go func() {
		if err := m.mailer.SendTeamInvite(inviteeEmail, inviterEmail, site.BusinessName, acceptURL); err != nil {
			slog.Error("send team invite email", "error", err)
		}
	}()

	return member, nil
}

// Accept binds a pending invite to the logged-in user who claimed it. The
// invite's email must match the accepting user's own account email — this
// stops someone else's invite link from being claimed by whoever happens to
// click it first.
func (m *Members) Accept(ctx context.Context, token string, userID uuid.UUID, userEmail string) (*domain.SiteMember, error) {
	invite, err := postgres.GetSiteMemberByToken(ctx, m.store.DB(), token)
	if err != nil {
		return nil, fmt.Errorf("load invite: %w", err)
	}
	if invite == nil {
		return nil, ErrInviteNotFound
	}
	if invite.Status == domain.SiteMemberStatusAccepted {
		if invite.UserID != nil && *invite.UserID == userID {
			return invite, nil
		}
		return nil, ErrInviteNotFound
	}
	if !strings.EqualFold(invite.Email, userEmail) {
		return nil, ErrInviteEmailMismatch
	}
	if err := postgres.AcceptSiteMemberInvite(ctx, m.store.DB(), invite.ID, userID); err != nil {
		return nil, fmt.Errorf("accept invite: %w", err)
	}
	invite.UserID = &userID
	invite.Status = domain.SiteMemberStatusAccepted
	return invite, nil
}

func (m *Members) List(ctx context.Context, siteID int) ([]domain.SiteMember, error) {
	return postgres.ListSiteMembersBySite(ctx, m.store.DB(), siteID)
}

// GetByToken looks up a pending/accepted invite by its raw token, for
// rendering the accept-invite confirmation page before the visitor submits.
func (m *Members) GetByToken(ctx context.Context, token string) (*domain.SiteMember, error) {
	return postgres.GetSiteMemberByToken(ctx, m.store.DB(), token)
}

// ListSitesByMember returns every site userID has accepted team access to,
// for the dashboard's sites list.
func (m *Members) ListSitesByMember(ctx context.Context, userID uuid.UUID) ([]domain.Site, error) {
	return postgres.ListSitesByMember(ctx, m.store.DB(), userID)
}

// Remove revokes a teammate's access (or withdraws a pending invite).
func (m *Members) Remove(ctx context.Context, siteID, memberID int) error {
	member, err := postgres.GetSiteMemberByID(ctx, m.store.DB(), memberID)
	if err != nil {
		return fmt.Errorf("load member: %w", err)
	}
	if member == nil || member.SiteID != siteID {
		return nil
	}
	return postgres.DeleteSiteMember(ctx, m.store.DB(), memberID)
}

// IsAcceptedMember satisfies middleware.MemberLoader, letting the ownership
// middleware admit accepted members alongside the site's owner.
func (m *Members) IsAcceptedMember(ctx context.Context, siteID int, userID uuid.UUID) (bool, error) {
	return postgres.IsAcceptedSiteMember(ctx, m.store.DB(), siteID, userID)
}

func generateInviteToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
