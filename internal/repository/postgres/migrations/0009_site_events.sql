-- Conversion events (call taps, WhatsApp taps, directions clicks, and
-- leads) — the actions that actually matter to a local business, tracked
-- alongside page views so the dashboard and analytics digest can surface
-- them instead of raw traffic noise.
CREATE TABLE IF NOT EXISTS site_events (
    id           SERIAL PRIMARY KEY,
    site_id      INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    kind         TEXT NOT NULL CHECK (kind IN ('call', 'whatsapp', 'directions', 'lead')),
    visitor_hash TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_site_events_site_id_created_at ON site_events(site_id, created_at);
