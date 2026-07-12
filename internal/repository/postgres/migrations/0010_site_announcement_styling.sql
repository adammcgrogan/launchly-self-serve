-- Lets owners style their announcement banner (info/promo/urgent tone),
-- attach an optional link/CTA, and lets visitors dismiss it.
ALTER TABLE site_announcements
    ADD COLUMN IF NOT EXISTS tone       TEXT NOT NULL DEFAULT 'info',
    ADD COLUMN IF NOT EXISTS link_url   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS link_label TEXT NOT NULL DEFAULT '';
