-- Lets an owner customize the contact-form confirmation: a replacement
-- thank-you message, and/or a redirect URL sent instead of showing it.
ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS thank_you_message TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS redirect_url TEXT NOT NULL DEFAULT '';
