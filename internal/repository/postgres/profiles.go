package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
)

// UpsertProfile creates the local profile row for a Supabase auth user on
// first login/signup, or refreshes the cached email on subsequent logins.
func UpsertProfile(ctx context.Context, q querier, id uuid.UUID, email string) (*domain.Profile, error) {
	var p domain.Profile
	err := q.QueryRowContext(ctx, `
		INSERT INTO profiles (id, email)
		VALUES ($1, $2)
		ON CONFLICT (id) DO UPDATE SET email = EXCLUDED.email
		RETURNING id, email, created_at
	`, id, email).Scan(&p.ID, &p.Email, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetProfile returns nil, nil if no profile exists for the given ID.
func GetProfile(ctx context.Context, q querier, id uuid.UUID) (*domain.Profile, error) {
	var p domain.Profile
	err := q.QueryRowContext(ctx,
		`SELECT id, email, created_at FROM profiles WHERE id = $1`, id,
	).Scan(&p.ID, &p.Email, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
