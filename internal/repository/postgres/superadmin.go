package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

const superadminAdminColumns = `id, email, password_hash, created_at`

func scanSuperadminAdmin(row *sql.Row) (*domain.SuperadminAdmin, error) {
	var a domain.SuperadminAdmin
	err := row.Scan(&a.ID, &a.Email, &a.PasswordHash, &a.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// CountSuperadminAdmins reports how many superadmin accounts exist — used
// to decide whether to bootstrap the first one from env vars on startup.
func CountSuperadminAdmins(ctx context.Context, q querier) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM superadmin_admins`).Scan(&n)
	return n, err
}

// CreateSuperadminAdmin inserts a new admin account. The caller is
// responsible for hashing password first.
func CreateSuperadminAdmin(ctx context.Context, q querier, email, passwordHash string) (*domain.SuperadminAdmin, error) {
	return scanSuperadminAdmin(q.QueryRowContext(ctx, `
		INSERT INTO superadmin_admins (email, password_hash)
		VALUES (lower($1), $2)
		RETURNING `+superadminAdminColumns, email, passwordHash))
}

func GetSuperadminAdminByEmail(ctx context.Context, q querier, email string) (*domain.SuperadminAdmin, error) {
	return scanSuperadminAdmin(q.QueryRowContext(ctx,
		`SELECT `+superadminAdminColumns+` FROM superadmin_admins WHERE lower(email) = lower($1)`, email))
}

// InsertSuperadminAuditLog records one superadmin login or action.
func InsertSuperadminAuditLog(ctx context.Context, q querier, adminEmail, action string, siteID *int, detail string) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO superadmin_audit_log (admin_email, action, site_id, detail)
		VALUES ($1, $2, $3, $4)
	`, adminEmail, action, siteID, detail)
	return err
}

const superadminAuditLogPageSize = 50

// ListSuperadminAuditLog returns the most recent audit log entries, newest
// first, paginated the same way SuperadminDashboard's site list is.
func ListSuperadminAuditLog(ctx context.Context, q querier, page int) ([]domain.SuperadminAuditLogEntry, int, error) {
	var total int
	if err := q.QueryRowContext(ctx, `SELECT count(*) FROM superadmin_audit_log`).Scan(&total); err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * superadminAuditLogPageSize
	rows, err := q.QueryContext(ctx, `
		SELECT id, admin_email, action, site_id, detail, created_at
		FROM superadmin_audit_log
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`, superadminAuditLogPageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []domain.SuperadminAuditLogEntry
	for rows.Next() {
		var e domain.SuperadminAuditLogEntry
		var siteID sql.NullInt64
		if err := rows.Scan(&e.ID, &e.AdminEmail, &e.Action, &siteID, &e.Detail, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		if siteID.Valid {
			id := int(siteID.Int64)
			e.SiteID = &id
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}
