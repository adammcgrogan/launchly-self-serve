-- Lets owners set a temporary banner on their site (closures, fully booked,
-- etc.) that auto-expires, without touching the full edit form.
CREATE TABLE IF NOT EXISTS site_announcements (
    site_id    INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    text       TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ
);
