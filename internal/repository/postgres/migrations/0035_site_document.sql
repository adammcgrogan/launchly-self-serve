-- Optional per-site downloadable document (e.g. a menu or brochure PDF),
-- rendered as a "Download our menu/brochure" button on the public site. See
-- #121. Rides on the same Storage upload mechanism as logo_url.
ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS document_title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS document_url TEXT NOT NULL DEFAULT '';
