package service

import (
	"context"
	"errors"
	"log/slog"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"golang.org/x/crypto/bcrypt"
)

// SuperadminAuditLogPageSize is how many audit log entries ListAuditLog
// returns per page.
const SuperadminAuditLogPageSize = 50

// ErrInvalidSuperadminCredentials is returned by Authenticate when the
// email/password pair doesn't match a superadmin account.
var ErrInvalidSuperadminCredentials = errors.New("invalid superadmin credentials")

// dummyBcryptHash is compared against on a not-found email so a login
// attempt for a nonexistent admin takes the same time as one for a real
// admin with a wrong password — otherwise the response-time difference
// (skip bcrypt entirely vs. run it) would let an attacker enumerate valid
// superadmin email addresses.
const dummyBcryptHash = "$2a$10$CwTycUXWue0Thq9StjUM0uJ8vRcSy4EAdY4hIWGvPZm5ZK5N4KA1O"

// Superadmin manages per-admin superadmin accounts and the audit log of
// superadmin actions (see #94) — replacing the old single shared
// SUPERADMIN_PASSWORD.
type Superadmin struct {
	store *postgres.Store
}

func NewSuperadmin(store *postgres.Store) *Superadmin {
	return &Superadmin{store: store}
}

// Bootstrap creates the first superadmin account from env-configured
// credentials, but only if no admin accounts exist yet. This is the only
// way to get an initial account onto a fresh environment without a manual
// DB insert; once at least one admin exists, bootstrapEmail/Password are
// ignored, so they're safe to leave set in Railway permanently. Additional
// admins beyond the first must be inserted directly (there's no
// self-serve "add admin" UI yet — out of scope for this issue).
func (s *Superadmin) Bootstrap(ctx context.Context, bootstrapEmail, bootstrapPassword string) error {
	if bootstrapEmail == "" || bootstrapPassword == "" {
		return nil
	}
	count, err := postgres.CountSuperadminAdmins(ctx, s.store.DB())
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(bootstrapPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if _, err := postgres.CreateSuperadminAdmin(ctx, s.store.DB(), bootstrapEmail, string(hash)); err != nil {
		return err
	}
	slog.Info("bootstrapped first superadmin account", "email", bootstrapEmail)
	return nil
}

// Authenticate checks email/password against the superadmin_admins table,
// returning ErrInvalidSuperadminCredentials on any mismatch (wrong email or
// wrong password — the caller shouldn't distinguish the two).
func (s *Superadmin) Authenticate(ctx context.Context, email, password string) error {
	admin, err := postgres.GetSuperadminAdminByEmail(ctx, s.store.DB(), email)
	if err != nil {
		return err
	}
	if admin == nil {
		bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
		return ErrInvalidSuperadminCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidSuperadminCredentials
	}
	return nil
}

// LogAction records a superadmin login or action in the audit log. siteID
// is nil for actions not tied to a specific site (e.g. login). Failures are
// logged but not returned — a logging failure shouldn't block the action
// itself or lock an admin out.
func (s *Superadmin) LogAction(ctx context.Context, adminEmail, action string, siteID *int, detail string) {
	if err := postgres.InsertSuperadminAuditLog(ctx, s.store.DB(), adminEmail, action, siteID, detail); err != nil {
		slog.Error("superadmin audit log insert failed", "error", err, "admin_email", adminEmail, "action", action)
	}
}

// ListAuditLog returns a page of audit log entries, newest first, and the
// total entry count (for pagination).
func (s *Superadmin) ListAuditLog(ctx context.Context, page int) ([]domain.SuperadminAuditLogEntry, int, error) {
	return postgres.ListSuperadminAuditLog(ctx, s.store.DB(), page)
}
