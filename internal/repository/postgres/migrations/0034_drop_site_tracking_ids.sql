-- GA4/Meta Pixel tracking support (added in 0019) is being removed entirely
-- per #175 — Launchly won't offer external third-party tracking on customer
-- sites, and the shipped snippets fired with no cookie-consent gating.
ALTER TABLE site_analytics_settings
    DROP COLUMN IF EXISTS ga_measurement_id,
    DROP COLUMN IF EXISTS meta_pixel_id;
