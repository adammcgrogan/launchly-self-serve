-- Replaces the single shared SUPERADMIN_PASSWORD with per-admin accounts
-- (see #94): each admin gets their own email + bcrypt password hash, and
-- every login and destructive superadmin action (unpublish/delete/edit) is
-- recorded in superadmin_audit_log with who did it and when.
CREATE TABLE IF NOT EXISTS superadmin_admins (
    id            SERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS superadmin_audit_log (
    id          BIGSERIAL PRIMARY KEY,
    admin_email TEXT NOT NULL,
    action      TEXT NOT NULL,
    site_id     INTEGER REFERENCES sites(id) ON DELETE SET NULL,
    detail      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_superadmin_audit_log_created_at ON superadmin_audit_log (created_at DESC);
