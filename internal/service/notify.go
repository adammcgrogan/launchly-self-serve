package service

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/google/uuid"
)

// notifyEmail resolves where owner-facing notifications (lead alerts, trial
// warnings, billing emails, analytics digests) should go: the authenticated
// account owner's login email takes priority, falling back to the site's
// public contact email. The public email is optional and, when left blank,
// must not silence notifications entirely.
func notifyEmail(ctx context.Context, store *postgres.Store, ownerUserID uuid.UUID, contactEmail string) string {
	profile, err := postgres.GetProfile(ctx, store.DB(), ownerUserID)
	if err == nil && profile != nil && profile.Email != "" {
		return profile.Email
	}
	return contactEmail
}

// resolveNotifyTarget loads a site and resolves where its owner-facing
// notifications should go, centralizing the "look up the site, look up its
// (possibly absent) public contact, then resolve notifyEmail" dance that used
// to be copy-pasted at every call site (#231). Returns a nil site (and empty
// email) without error if the site itself no longer exists.
func resolveNotifyTarget(ctx context.Context, store *postgres.Store, siteID int) (site *domain.Site, notifyTo string, err error) {
	site, err = postgres.GetSiteByID(ctx, store.DB(), siteID)
	if err != nil || site == nil {
		return site, "", err
	}
	contact, err := postgres.GetSiteContact(ctx, store.DB(), siteID)
	if err != nil {
		return site, "", err
	}
	contactEmail := ""
	if contact != nil {
		contactEmail = contact.Email
	}
	return site, notifyEmail(ctx, store, site.OwnerUserID, contactEmail), nil
}
