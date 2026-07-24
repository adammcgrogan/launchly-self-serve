-- Team invite links never expired: a leaked or forgotten invite token kept
-- working indefinitely. Give pending invites a bounded 7-day window (mirrors
-- invited_at, so existing rows backfill to invited_at + 7 days rather than
-- all expiring at once on deploy).
ALTER TABLE site_members ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '7 days');
UPDATE site_members SET expires_at = invited_at + interval '7 days' WHERE status = 'pending';
