-- Umami analytics was wired into config/DB/templates but never had a UI to
-- configure it, so the column was always empty. Dropping the dead wiring.
ALTER TABLE site_analytics_settings
    DROP COLUMN IF EXISTS umami_website_id;
