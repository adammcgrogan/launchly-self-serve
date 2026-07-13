-- Lets an owner set an exact brand hex colour that overrides the accent
-- colour of their chosen preset palette.
ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS brand_color TEXT NOT NULL DEFAULT '';
