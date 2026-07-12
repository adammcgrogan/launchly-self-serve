-- Tracks an owner's follow-up progress on a lead (new / contacted / won / lost).
ALTER TABLE leads ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'new';
