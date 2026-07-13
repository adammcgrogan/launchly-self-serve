package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
)

// ReplaceSiteStaffMembers overwrites all staff members for a site.
func ReplaceSiteStaffMembers(ctx context.Context, q querier, siteID int, members []domain.StaffMember) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM site_staff_members WHERE site_id = $1`, siteID); err != nil {
		return err
	}
	for i, m := range members {
		if m.Name == "" {
			continue
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO site_staff_members (site_id, name, role, photo_url, bio, sort_order) VALUES ($1, $2, $3, $4, $5, $6)`,
			siteID, m.Name, m.Role, m.PhotoURL, m.Bio, i,
		); err != nil {
			return err
		}
	}
	return nil
}

func GetSiteStaffMembers(ctx context.Context, q querier, siteID int) ([]domain.StaffMember, error) {
	rows, err := q.QueryContext(ctx,
		`SELECT id, site_id, name, role, photo_url, bio, sort_order FROM site_staff_members WHERE site_id = $1 ORDER BY sort_order`, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.StaffMember
	for rows.Next() {
		var m domain.StaffMember
		if err := rows.Scan(&m.ID, &m.SiteID, &m.Name, &m.Role, &m.PhotoURL, &m.Bio, &m.SortOrder); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
