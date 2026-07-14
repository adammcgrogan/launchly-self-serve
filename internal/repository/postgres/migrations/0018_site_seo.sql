-- Lets an owner override the page <title>, meta description, and social
-- share (Open Graph) image per site. All optional — the public templates
-- fall back to business name/tagline/logo when these are empty.
ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS meta_title       TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS meta_description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS og_image_url     TEXT NOT NULL DEFAULT '';
