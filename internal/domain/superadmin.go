package domain

import "time"

// SuperadminAdmin is a per-person superadmin account (see #94), replacing
// the single shared SUPERADMIN_PASSWORD. PasswordHash is a bcrypt hash,
// consistent with how the rest of the codebase would hash a
// locally-managed password (customer accounts are hashed by Supabase Auth
// instead, so this is the first local password store in the app).
type SuperadminAdmin struct {
	ID           int
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// SuperadminAuditLogEntry records a single superadmin login or action —
// who did it, what they did, which site (if any) it targeted, and when.
type SuperadminAuditLogEntry struct {
	ID         int64
	AdminEmail string
	Action     string
	SiteID     *int
	Detail     string
	CreatedAt  time.Time
}
