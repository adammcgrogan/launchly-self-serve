-- Team access: lets a site owner invite a teammate (front-desk staff, a
-- partner) to manage a site's content and leads without sharing the owner's
-- login. A row starts 'pending' (invited by email, user_id unset) and
-- becomes 'accepted' once the invitee signs up/logs in and claims the
-- invite token. Only one role exists today ("member") — destructive and
-- billing actions stay owner-only, gated by sites.owner_user_id directly.
CREATE TABLE IF NOT EXISTS site_members (
    id           SERIAL PRIMARY KEY,
    site_id      INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    user_id      UUID REFERENCES profiles(id) ON DELETE CASCADE,
    email        TEXT NOT NULL,
    role         TEXT NOT NULL DEFAULT 'member',
    status       TEXT NOT NULL DEFAULT 'pending',
    invite_token TEXT NOT NULL DEFAULT '',
    invited_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    accepted_at  TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_site_members_site_id ON site_members(site_id);
CREATE INDEX IF NOT EXISTS idx_site_members_user_id ON site_members(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_site_members_site_id_email ON site_members(site_id, lower(email));
CREATE UNIQUE INDEX IF NOT EXISTS idx_site_members_invite_token ON site_members(invite_token) WHERE invite_token <> '';
