package postgres

import (
	"context"
	"database/sql"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

func CreateLead(ctx context.Context, q querier, lead *domain.Lead) error {
	return q.QueryRowContext(ctx, `
		INSERT INTO leads (site_id, name, email, phone, message, service_label, preferred_time)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`, lead.SiteID, lead.Name, lead.Email, lead.Phone, lead.Message, lead.ServiceLabel, lead.PreferredTime).Scan(&lead.ID, &lead.CreatedAt)
}

func ListLeadsBySite(ctx context.Context, q querier, siteID int) ([]domain.Lead, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, site_id, name, email, phone, message, service_label, preferred_time, status, created_at FROM leads WHERE site_id = $1 ORDER BY created_at DESC`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Lead
	for rows.Next() {
		var l domain.Lead
		if err := rows.Scan(&l.ID, &l.SiteID, &l.Name, &l.Email, &l.Phone, &l.Message, &l.ServiceLabel, &l.PreferredTime, &l.Status, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// UpdateLeadStatus sets a lead's status, scoped to siteID so a caller can't
// update a lead belonging to a different site. Returns sql.ErrNoRows if the
// lead doesn't exist or isn't on that site.
func UpdateLeadStatus(ctx context.Context, q querier, siteID, leadID int, status domain.LeadStatus) error {
	res, err := q.ExecContext(ctx,
		`UPDATE leads SET status = $1 WHERE id = $2 AND site_id = $3`, status, leadID, siteID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
