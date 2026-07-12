-- Extends services from bare labels to a real service menu: an optional
-- description and a free-text price (e.g. "from £25", "£40/hr", "POA").
ALTER TABLE site_services
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS price_text  TEXT NOT NULL DEFAULT '';
