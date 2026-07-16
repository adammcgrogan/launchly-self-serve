-- Date-scoped opening-hours overrides (e.g. "closed 24-26 Dec", "different
-- hours on a bank holiday") layered on top of the weekly site_business_hours
-- rows from 0008 — see domain.SiteAggregate.OpenNow and OpenDays.
CREATE TABLE site_special_hours (
    id        SERIAL PRIMARY KEY,
    site_id   INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    date      DATE NOT NULL,
    label     TEXT NOT NULL DEFAULT '', -- e.g. "Christmas Day"
    opens_at  TEXT NOT NULL DEFAULT '', -- "HH:MM", 24-hour, empty when closed
    closes_at TEXT NOT NULL DEFAULT '',
    closed    BOOLEAN NOT NULL DEFAULT FALSE
);
CREATE INDEX idx_site_special_hours_site_id ON site_special_hours(site_id);
