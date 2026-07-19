package postgres

import (
	"context"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/lib/pq"
)

// CreateLeadNote inserts a follow-up note for a lead, scoped to siteID via
// the INSERT ... SELECT so a caller can't attach a note to a lead belonging
// to a different site. Returns sql.ErrNoRows if the lead doesn't exist or
// isn't on that site.
func CreateLeadNote(ctx context.Context, q querier, siteID, leadID int, body string) (*domain.LeadNote, error) {
	var n domain.LeadNote
	err := q.QueryRowContext(ctx, `
		INSERT INTO lead_notes (lead_id, body)
		SELECT id, $3 FROM leads WHERE id = $2 AND site_id = $1
		RETURNING id, lead_id, body, created_at
	`, siteID, leadID, body).Scan(&n.ID, &n.LeadID, &n.Body, &n.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// ListLeadNotesByLeadIDs batch-loads notes for a page of leads in one query
// (instead of one query per lead), oldest first, grouped by lead ID.
func ListLeadNotesByLeadIDs(ctx context.Context, q querier, leadIDs []int) (map[int][]domain.LeadNote, error) {
	notesByLead := map[int][]domain.LeadNote{}
	if len(leadIDs) == 0 {
		return notesByLead, nil
	}
	rows, err := q.QueryContext(ctx,
		`SELECT id, lead_id, body, created_at FROM lead_notes WHERE lead_id = ANY($1) ORDER BY created_at ASC`,
		pq.Array(leadIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n domain.LeadNote
		if err := rows.Scan(&n.ID, &n.LeadID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		notesByLead[n.LeadID] = append(notesByLead[n.LeadID], n)
	}
	return notesByLead, rows.Err()
}
