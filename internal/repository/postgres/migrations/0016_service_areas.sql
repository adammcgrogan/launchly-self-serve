-- Adds an editable "areas we serve" list for mobile/at-home trades with no
-- single storefront: town/region strings shown on the site and fed into the
-- LocalBusiness areaServed structured data for local SEO.
CREATE TABLE IF NOT EXISTS site_service_areas (
    id         SERIAL PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    area       TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_site_service_areas_site_id ON site_service_areas(site_id);
