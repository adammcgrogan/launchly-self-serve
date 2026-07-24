-- Tracks whether the owner-notification email for a lead failed to send, so
-- the dashboard can surface it instead of the failure only being logged.
ALTER TABLE leads ADD COLUMN IF NOT EXISTS notify_failed BOOLEAN NOT NULL DEFAULT false;
