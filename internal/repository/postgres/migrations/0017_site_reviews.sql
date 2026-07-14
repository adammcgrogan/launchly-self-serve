-- Adds an owner-entered review rating badge: a display star rating, review
-- count, and a "leave us a review" link (e.g. the business's Google review
-- page). 1:1 with a site. Rating is stored as text so an unset badge is a
-- plain empty string rather than an ambiguous 0.
CREATE TABLE IF NOT EXISTS site_reviews (
    site_id      INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    rating       TEXT NOT NULL DEFAULT '',
    review_count INTEGER NOT NULL DEFAULT 0,
    review_url   TEXT NOT NULL DEFAULT ''
);
