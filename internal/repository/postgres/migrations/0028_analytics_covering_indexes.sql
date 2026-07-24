-- GetSiteStats groups by referrer/kind and counts distinct visitor_hash
-- within the (site_id, created_at) range these indexes already narrow down
-- to; including those columns lets Postgres satisfy the aggregates from the
-- index alone instead of fetching each matching row from the heap.
DROP INDEX IF EXISTS idx_page_views_site_id_created_at;
CREATE INDEX IF NOT EXISTS idx_page_views_site_id_created_at ON page_views(site_id, created_at) INCLUDE (referrer, visitor_hash);

DROP INDEX IF EXISTS idx_site_events_site_id_created_at;
CREATE INDEX IF NOT EXISTS idx_site_events_site_id_created_at ON site_events(site_id, created_at) INCLUDE (kind);
