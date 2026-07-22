-- Demo sites are Launchly-owned showcase sites (one per template) that seed
-- the /templates gallery with a live example, independent of real customer
-- opt-in. Flagged so they can be excluded from platform stats.
ALTER TABLE sites ADD COLUMN IF NOT EXISTS is_demo BOOLEAN NOT NULL DEFAULT false;
