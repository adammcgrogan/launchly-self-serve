// Package service holds business logic: it orchestrates the repository,
// supabase, email, and payment packages. Handlers call into here and never
// touch the database or Supabase directly.
package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/adammcgrogan/launchly-self-serve/internal/email"
	"github.com/adammcgrogan/launchly-self-serve/internal/repository/postgres"
	"github.com/adammcgrogan/launchly-self-serve/internal/supabase"
	"github.com/google/uuid"
)

// Accounts handles signup, login, and session lifecycle. Credential storage
// and verification/reset emails are Supabase's responsibility; this only
// keeps our local `profiles` row in sync and sends our own welcome email.
type Accounts struct {
	store   *postgres.Store
	supa    *supabase.Client
	mailer  *email.Client
	baseURL string
}

func NewAccounts(store *postgres.Store, supa *supabase.Client, mailer *email.Client, baseURL string) *Accounts {
	return &Accounts{store: store, supa: supa, mailer: mailer, baseURL: baseURL}
}

// SignUp creates a new Supabase auth user, creates the matching local
// profile row, and fires a welcome email. If the Supabase project requires
// email confirmation, the returned session will have no access token —
// callers should treat that as "check your email", not an error.
func (a *Accounts) SignUp(ctx context.Context, emailAddr, password string) (*supabase.Session, error) {
	sess, err := a.supa.SignUp(ctx, emailAddr, password)
	if err != nil {
		return nil, err
	}
	if _, err := postgres.UpsertProfile(ctx, a.store.DB(), sess.UserID, sess.Email); err != nil {
		return nil, fmt.Errorf("create profile: %w", err)
	}

	go func() {
		if err := a.mailer.SendWelcomeEmail(sess.Email, a.baseURL+"/dashboard"); err != nil {
			slog.Error("send welcome email", "error", err)
		}
	}()

	return sess, nil
}

// Login authenticates against Supabase and refreshes the cached profile email.
func (a *Accounts) Login(ctx context.Context, emailAddr, password string) (*supabase.Session, error) {
	sess, err := a.supa.SignInWithPassword(ctx, emailAddr, password)
	if err != nil {
		return nil, err
	}
	if _, err := postgres.UpsertProfile(ctx, a.store.DB(), sess.UserID, sess.Email); err != nil {
		return nil, fmt.Errorf("sync profile: %w", err)
	}
	return sess, nil
}

// Logout invalidates the session on Supabase's side. Errors are non-fatal —
// the caller should clear the local session cookie regardless.
func (a *Accounts) Logout(ctx context.Context, accessToken string) error {
	return a.supa.SignOut(ctx, accessToken)
}

func (a *Accounts) RequestPasswordReset(ctx context.Context, emailAddr string) error {
	return a.supa.SendPasswordReset(ctx, emailAddr)
}

func (a *Accounts) ResendVerificationEmail(ctx context.Context, emailAddr string) error {
	return a.supa.ResendVerificationEmail(ctx, emailAddr)
}

// UpdatePassword completes a password-reset flow: accessToken is the
// recovery-scoped token from the link Supabase emailed the user.
func (a *Accounts) UpdatePassword(ctx context.Context, accessToken, newPassword string) error {
	return a.supa.UpdatePassword(ctx, accessToken, newPassword)
}

func (a *Accounts) GetProfile(ctx context.Context, userID uuid.UUID) (*domain.Profile, error) {
	return postgres.GetProfile(ctx, a.store.DB(), userID)
}
