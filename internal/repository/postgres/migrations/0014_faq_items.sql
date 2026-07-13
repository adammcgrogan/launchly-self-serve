-- Adds an editable FAQ section: question/answer pairs shown on a site.
CREATE TABLE IF NOT EXISTS site_faq_items (
    id         SERIAL PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    question   TEXT NOT NULL DEFAULT '',
    answer     TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_site_faq_items_site_id ON site_faq_items(site_id);
