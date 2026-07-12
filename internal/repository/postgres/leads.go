package postgres

import (
	"context"

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
		`SELECT id, site_id, name, email, phone, message, service_label, preferred_time, created_at FROM leads WHERE site_id = $1 ORDER BY created_at DESC`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Lead
	for rows.Next() {
		var l domain.Lead
		if err := rows.Scan(&l.ID, &l.SiteID, &l.Name, &l.Email, &l.Phone, &l.Message, &l.ServiceLabel, &l.PreferredTime, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
