package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
)

const siteColumns = `id, owner_user_id, slug, business_name, tagline, about,
	template_id, palette, heading_font, status, created_at, published_at, updated_at`

func scanSite(row *sql.Row) (*domain.Site, error) {
	var s domain.Site
	err := row.Scan(
		&s.ID, &s.OwnerUserID, &s.Slug, &s.BusinessName, &s.Tagline, &s.About,
		&s.TemplateID, &s.Palette, &s.HeadingFont, &s.Status, &s.CreatedAt, &s.PublishedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func scanSiteRows(rows *sql.Rows) (*domain.Site, error) {
	var s domain.Site
	err := rows.Scan(
		&s.ID, &s.OwnerUserID, &s.Slug, &s.BusinessName, &s.Tagline, &s.About,
		&s.TemplateID, &s.Palette, &s.HeadingFont, &s.Status, &s.CreatedAt, &s.PublishedAt, &s.UpdatedAt,
	)
	return &s, err
}

// CreateSite inserts a site's core row. Status is set to live and
// published_at to now — sites go live immediately, there is no draft/review
// step in the self-serve flow.
func CreateSite(ctx context.Context, q querier, site *domain.Site) (int, error) {
	now := time.Now().UTC()
	err := q.QueryRowContext(ctx, `
		INSERT INTO sites (owner_user_id, slug, business_name, tagline, about,
		                   template_id, palette, heading_font, status, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'live', $9)
		RETURNING id
	`, site.OwnerUserID, site.Slug, site.BusinessName, site.Tagline, site.About,
		site.TemplateID, site.Palette, site.HeadingFont, now,
	).Scan(&site.ID)
	return site.ID, err
}

func GetSiteByID(ctx context.Context, q querier, id int) (*domain.Site, error) {
	return scanSite(q.QueryRowContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE id = $1`, id))
}

func GetSiteBySlug(ctx context.Context, q querier, slug string) (*domain.Site, error) {
	return scanSite(q.QueryRowContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE slug = $1`, slug))
}

func ListSitesByOwner(ctx context.Context, q querier, ownerID uuid.UUID) ([]domain.Site, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE owner_user_id = $1 ORDER BY created_at DESC`, ownerID)
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

// ListAllSites returns every site, newest first — used by the superadmin view.
func ListAllSites(ctx context.Context, q querier) ([]domain.Site, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+siteColumns+` FROM sites ORDER BY created_at DESC`)
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

// UpdateSiteContent saves the editable core fields (not appearance/template/status).
func UpdateSiteContent(ctx context.Context, q querier, site *domain.Site) error {
	_, err := q.ExecContext(ctx, `
		UPDATE sites SET business_name = $1, tagline = $2, about = $3, updated_at = now()
		WHERE id = $4
	`, site.BusinessName, site.Tagline, site.About, site.ID)
	return err
}

func UpdateSiteAppearance(ctx context.Context, q querier, id int, palette, headingFont string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET palette = $1, heading_font = $2, updated_at = now() WHERE id = $3`, palette, headingFont, id)
	return err
}

func UpdateSiteTemplate(ctx context.Context, q querier, id int, templateID string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET template_id = $1, updated_at = now() WHERE id = $2`, templateID, id)
	return err
}

func SetSiteStatus(ctx context.Context, q querier, id int, status domain.SiteStatus) error {
	if status == domain.SiteStatusLive {
		_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'live', published_at = now(), updated_at = now() WHERE id = $1`, id)
		return err
	}
	_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'draft', updated_at = now() WHERE id = $1`, id)
	return err
}

func DeleteSite(ctx context.Context, q querier, id int) error {
	_, err := q.ExecContext(ctx, `DELETE FROM sites WHERE id = $1`, id)
	return err
}
