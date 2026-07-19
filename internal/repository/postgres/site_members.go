package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
)

const siteMemberColumns = `id, site_id, user_id, email, role, status, invite_token, invited_at, accepted_at`

func scanSiteMember(row *sql.Row) (*domain.SiteMember, error) {
	var m domain.SiteMember
	var userID uuid.NullUUID
	err := row.Scan(&m.ID, &m.SiteID, &userID, &m.Email, &m.Role, &m.Status, &m.InviteToken, &m.InvitedAt, &m.AcceptedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		m.UserID = &userID.UUID
	}
	return &m, nil
}

func scanSiteMemberRows(rows *sql.Rows) (*domain.SiteMember, error) {
	var m domain.SiteMember
	var userID uuid.NullUUID
	err := rows.Scan(&m.ID, &m.SiteID, &userID, &m.Email, &m.Role, &m.Status, &m.InviteToken, &m.InvitedAt, &m.AcceptedAt)
	if err != nil {
		return nil, err
	}
	if userID.Valid {
		m.UserID = &userID.UUID
	}
	return &m, nil
}

// CreateSiteMemberInvite inserts a pending invite row. The caller is
// responsible for checking there isn't already a member/invite for this
// site+email first (the unique index is the backstop).
func CreateSiteMemberInvite(ctx context.Context, q querier, siteID int, email, inviteToken string) (*domain.SiteMember, error) {
	return scanSiteMember(q.QueryRowContext(ctx, `
		INSERT INTO site_members (site_id, email, invite_token)
		VALUES ($1, $2, $3)
		RETURNING `+siteMemberColumns, siteID, email, inviteToken))
}

func GetSiteMemberByToken(ctx context.Context, q querier, token string) (*domain.SiteMember, error) {
	return scanSiteMember(q.QueryRowContext(ctx, `SELECT `+siteMemberColumns+` FROM site_members WHERE invite_token = $1`, token))
}

func GetSiteMemberByID(ctx context.Context, q querier, id int) (*domain.SiteMember, error) {
	return scanSiteMember(q.QueryRowContext(ctx, `SELECT `+siteMemberColumns+` FROM site_members WHERE id = $1`, id))
}

func GetSiteMemberBySiteAndEmail(ctx context.Context, q querier, siteID int, email string) (*domain.SiteMember, error) {
	return scanSiteMember(q.QueryRowContext(ctx, `SELECT `+siteMemberColumns+` FROM site_members WHERE site_id = $1 AND lower(email) = lower($2)`, siteID, email))
}

// AcceptSiteMemberInvite binds a pending invite to the user who claimed it.
func AcceptSiteMemberInvite(ctx context.Context, q querier, id int, userID uuid.UUID) error {
	_, err := q.ExecContext(ctx, `
		UPDATE site_members SET user_id = $1, status = 'accepted', accepted_at = now()
		WHERE id = $2
	`, userID, id)
	return err
}

func ListSiteMembersBySite(ctx context.Context, q querier, siteID int) ([]domain.SiteMember, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+siteMemberColumns+` FROM site_members WHERE site_id = $1 ORDER BY invited_at DESC`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []domain.SiteMember
	for rows.Next() {
		m, err := scanSiteMemberRows(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *m)
	}
	return members, rows.Err()
}

// IsAcceptedSiteMember reports whether userID has accepted membership on
// siteID — used by the ownership middleware to admit members alongside the
// owner.
func IsAcceptedSiteMember(ctx context.Context, q querier, siteID int, userID uuid.UUID) (bool, error) {
	var ok bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM site_members WHERE site_id = $1 AND user_id = $2 AND status = 'accepted')
	`, siteID, userID).Scan(&ok)
	return ok, err
}

func DeleteSiteMember(ctx context.Context, q querier, id int) error {
	_, err := q.ExecContext(ctx, `DELETE FROM site_members WHERE id = $1`, id)
	return err
}

// ListSitesByMember returns every site userID has accepted team access to
// (not sites they own), for the dashboard's sites list.
func ListSitesByMember(ctx context.Context, q querier, userID uuid.UUID) ([]domain.Site, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT `+siteColumns+` FROM sites
		WHERE id IN (SELECT site_id FROM site_members WHERE user_id = $1 AND status = 'accepted')
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sites []domain.Site
	for rows.Next() {
		s, err := scanSiteRows(rows)
		if err != nil {
			return nil, err
		}
		sites = append(sites, *s)
	}
	return sites, rows.Err()
}
