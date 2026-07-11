package service

import (
	"context"

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
