-- Lets owners rename a site's subdomain while keeping the old address
-- working: renames are recorded here and 301-redirected to the current slug.
CREATE TABLE IF NOT EXISTS slug_redirects (
    old_slug   TEXT PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_slug_redirects_site_id ON slug_redirects(site_id);

ALTER TABLE sites ADD COLUMN IF NOT EXISTS slug_changed_at TIMESTAMPTZ;
