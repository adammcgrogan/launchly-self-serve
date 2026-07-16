-- Lets an owner add an optional promo/walkthrough video (YouTube or Vimeo)
-- shown as a privacy-friendly click-to-load embed on their site.
ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS video_url TEXT NOT NULL DEFAULT '';
