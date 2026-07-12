-- Structured per-day opening hours (weekday/opens/closes/closed) replace the
-- old free-text label/hours-text rows, so the site can compute a live
-- "Open now" badge and emit real openingHoursSpecification JSON-LD — neither
-- is possible from a free-text line like "Mon-Fri | 9am-5pm".
DROP TABLE IF EXISTS site_business_hours;
CREATE TABLE site_business_hours (
    id        SERIAL PRIMARY KEY,
    site_id   INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    weekday   SMALLINT NOT NULL, -- 0=Sunday .. 6=Saturday, matches Go's time.Weekday
    opens_at  TEXT NOT NULL DEFAULT '', -- "HH:MM", 24-hour
    closes_at TEXT NOT NULL DEFAULT '',
    closed    BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_site_business_hours_site_id ON site_business_hours(site_id);

-- IANA zone opening hours (and the "Open now" badge) are evaluated in.
ALTER TABLE sites ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT 'Europe/London';
