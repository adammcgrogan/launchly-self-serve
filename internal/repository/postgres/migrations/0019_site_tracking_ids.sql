-- Lets Pro sites add their own Google Analytics 4 measurement ID and Meta
-- (Facebook) Pixel ID, rendered as tracking snippets on the public site so
-- owners can measure conversions in tools they already use.
ALTER TABLE site_analytics_settings
    ADD COLUMN IF NOT EXISTS ga_measurement_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS meta_pixel_id     TEXT NOT NULL DEFAULT '';
