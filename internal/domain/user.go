package domain

import (
	"time"

	"github.com/google/uuid"
)

// Profile is the application's view of a Supabase-authenticated user.
// Credentials, sessions, and verification state live in Supabase's own
// auth.users schema — this is just the app-side row keyed by that user's ID.
type Profile struct {
	ID        uuid.UUID
	Email     string
	CreatedAt time.Time
}
