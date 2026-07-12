package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/adammcgrogan/launchly-self-serve/internal/domain"
	"github.com/google/uuid"
)

const siteColumns = `id, owner_user_id, slug, business_name, tagline, about, logo_url, cta_text,
	template_id, form_type, palette, heading_font, status, created_at, published_at, updated_at, slug_changed_at,
	custom_domain, custom_domain_status, custom_domain_cf_id, custom_domain_added_at, timezone`

func scanSite(row *sql.Row) (*domain.Site, error) {
	var s domain.Site
	var customDomain, customDomainCFID sql.NullString
	err := row.Scan(
		&s.ID, &s.OwnerUserID, &s.Slug, &s.BusinessName, &s.Tagline, &s.About, &s.LogoURL, &s.CTAText,
		&s.TemplateID, &s.FormType, &s.Palette, &s.HeadingFont, &s.Status, &s.CreatedAt, &s.PublishedAt, &s.UpdatedAt, &s.SlugChangedAt,
		&customDomain, &s.CustomDomainStatus, &customDomainCFID, &s.CustomDomainAddedAt, &s.Timezone,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.CustomDomain = customDomain.String
	s.CustomDomainCFID = customDomainCFID.String
	return &s, nil
}

func scanSiteRows(rows *sql.Rows) (*domain.Site, error) {
	var s domain.Site
	var customDomain, customDomainCFID sql.NullString
	err := rows.Scan(
		&s.ID, &s.OwnerUserID, &s.Slug, &s.BusinessName, &s.Tagline, &s.About, &s.LogoURL, &s.CTAText,
		&s.TemplateID, &s.FormType, &s.Palette, &s.HeadingFont, &s.Status, &s.CreatedAt, &s.PublishedAt, &s.UpdatedAt, &s.SlugChangedAt,
		&customDomain, &s.CustomDomainStatus, &customDomainCFID, &s.CustomDomainAddedAt, &s.Timezone,
	)
	s.CustomDomain = customDomain.String
	s.CustomDomainCFID = customDomainCFID.String
	return &s, err
}

// CreateSite inserts a site's core row. Status is set to live and
// published_at to now — sites go live immediately, there is no draft/review
// step in the self-serve flow.
func CreateSite(ctx context.Context, q querier, site *domain.Site) (int, error) {
	now := time.Now().UTC()
	err := q.QueryRowContext(ctx, `
		INSERT INTO sites (owner_user_id, slug, business_name, tagline, about, logo_url, cta_text,
		                   template_id, palette, heading_font, status, published_at, timezone)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'live', $11, $12)
		RETURNING id
	`, site.OwnerUserID, site.Slug, site.BusinessName, site.Tagline, site.About, site.LogoURL, site.CTAText,
		site.TemplateID, site.Palette, site.HeadingFont, now, site.Timezone,
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

// ListLiveSites returns every published site, for the public sitemap.
func ListLiveSites(ctx context.Context, q querier) ([]domain.Site, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+siteColumns+` FROM sites WHERE status = 'live' ORDER BY published_at DESC`)
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
		UPDATE sites SET business_name = $1, tagline = $2, about = $3, logo_url = $4, cta_text = $5, timezone = $6, updated_at = now()
		WHERE id = $7
	`, site.BusinessName, site.Tagline, site.About, site.LogoURL, site.CTAText, site.Timezone, site.ID)
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

func UpdateSiteFormType(ctx context.Context, q querier, id int, formType domain.FormType) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET form_type = $1, updated_at = now() WHERE id = $2`, formType, id)
	return err
}

// RenameSiteSlug updates a site's live slug and stamps slug_changed_at, used
// to enforce the once-per-day rename limit.
func RenameSiteSlug(ctx context.Context, q querier, id int, slug string) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET slug = $1, slug_changed_at = now(), updated_at = now() WHERE id = $2`, slug, id)
	return err
}

// SlugInUse reports whether slug is already a live site's slug or a
// redirect's old slug, so renames can't collide with either.
func SlugInUse(ctx context.Context, q querier, slug string) (bool, error) {
	var inUse bool
	err := q.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM sites WHERE slug = $1)
		OR EXISTS(SELECT 1 FROM slug_redirects WHERE old_slug = $1)
	`, slug).Scan(&inUse)
	return inUse, err
}

func SetSiteStatus(ctx context.Context, q querier, id int, status domain.SiteStatus) error {
	switch status {
	case domain.SiteStatusLive:
		_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'live', published_at = now(), updated_at = now() WHERE id = $1`, id)
		return err
	case domain.SiteStatusPaused:
		_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'paused', updated_at = now() WHERE id = $1`, id)
		return err
	default:
		_, err := q.ExecContext(ctx, `UPDATE sites SET status = 'draft', updated_at = now() WHERE id = $1`, id)
		return err
	}
}

func DeleteSite(ctx context.Context, q querier, id int) error {
	_, err := q.ExecContext(ctx, `DELETE FROM sites WHERE id = $1`, id)
	return err
}

// GetSiteByCustomDomain looks up the site that owns an active custom
// domain, for routing requests whose Host isn't a *.DOMAIN subdomain.
func GetSiteByCustomDomain(ctx context.Context, q querier, host string) (*domain.Site, error) {
	return scanSite(q.QueryRowContext(ctx,
		`SELECT `+siteColumns+` FROM sites WHERE custom_domain = $1 AND custom_domain_status = 'active'`, host))
}

// CustomDomainInUse reports whether customDomain is already attached to any site.
func CustomDomainInUse(ctx context.Context, q querier, customDomain string) (bool, error) {
	var inUse bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sites WHERE custom_domain = $1)`, customDomain).Scan(&inUse)
	return inUse, err
}

// SetCustomDomain attaches a pending custom domain to a site.
func SetCustomDomain(ctx context.Context, q querier, id int, customDomain, cfID string) error {
	_, err := q.ExecContext(ctx, `
		UPDATE sites
		SET custom_domain = $1, custom_domain_status = 'pending', custom_domain_cf_id = $2,
		    custom_domain_added_at = now(), updated_at = now()
		WHERE id = $3
	`, customDomain, cfID, id)
	return err
}

// UpdateCustomDomainStatus updates a site's custom domain verification status.
func UpdateCustomDomainStatus(ctx context.Context, q querier, id int, status domain.CustomDomainStatus) error {
	_, err := q.ExecContext(ctx, `UPDATE sites SET custom_domain_status = $1, updated_at = now() WHERE id = $2`, status, id)
	return err
}

// ClearCustomDomain removes a site's custom domain entirely.
func ClearCustomDomain(ctx context.Context, q querier, id int) error {
	_, err := q.ExecContext(ctx, `
		UPDATE sites
		SET custom_domain = NULL, custom_domain_status = 'none', custom_domain_cf_id = NULL,
		    custom_domain_added_at = NULL, updated_at = now()
		WHERE id = $1
	`, id)
	return err
}
