-- Core schema for launchly-self-serve.
--
-- User credentials/sessions live in Supabase's own `auth.users` schema.
-- `profiles` is our app-side row per Supabase auth user; everything else
-- hangs off `sites`, split into one table per logical group so no single
-- table grows into a 25-column catch-all.

CREATE TABLE IF NOT EXISTS profiles (
    id         UUID PRIMARY KEY REFERENCES auth.users(id) ON DELETE CASCADE,
    email      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sites (
    id            SERIAL PRIMARY KEY,
    owner_user_id UUID NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    slug          TEXT NOT NULL UNIQUE,
    business_name TEXT NOT NULL,
    tagline       TEXT NOT NULL DEFAULT '',
    about         TEXT NOT NULL DEFAULT '',
    logo_url      TEXT NOT NULL DEFAULT '',
    cta_text      TEXT NOT NULL DEFAULT '',
    template_id   TEXT NOT NULL,
    palette       TEXT NOT NULL DEFAULT '',
    heading_font  TEXT NOT NULL DEFAULT '',
    status        TEXT NOT NULL DEFAULT 'live',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at  TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sites_owner_user_id ON sites(owner_user_id);

CREATE TABLE IF NOT EXISTS site_contact (
    site_id       INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    phone         TEXT NOT NULL DEFAULT '',
    email         TEXT NOT NULL DEFAULT '',
    address       TEXT NOT NULL DEFAULT '',
    location      TEXT NOT NULL DEFAULT '',
    map_url       TEXT NOT NULL DEFAULT '',
    map_embed_url TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS site_billing (
    site_id                      INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    plan                         TEXT NOT NULL DEFAULT 'starter',
    payment_status               TEXT NOT NULL DEFAULT 'trialing',
    stripe_customer_id           TEXT NOT NULL DEFAULT '',
    stripe_session_id            TEXT NOT NULL DEFAULT '',
    stripe_subscription_id       TEXT NOT NULL DEFAULT '',
    paid_at                      TIMESTAMPTZ,
    trial_ends_at                TIMESTAMPTZ,
    trial_reminder_sent_at       TIMESTAMPTZ,
    trial_final_reminder_sent_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_site_billing_stripe_session_id
    ON site_billing(stripe_session_id) WHERE stripe_session_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_site_billing_stripe_subscription_id
    ON site_billing(stripe_subscription_id) WHERE stripe_subscription_id <> '';

CREATE TABLE IF NOT EXISTS site_analytics_settings (
    site_id                INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    umami_website_id       TEXT NOT NULL DEFAULT '',
    analytics_frequency    TEXT NOT NULL DEFAULT 'off',
    analytics_last_sent_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS site_social_links (
    id       SERIAL PRIMARY KEY,
    site_id  INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    platform TEXT NOT NULL,
    url      TEXT NOT NULL,
    UNIQUE (site_id, platform)
);

CREATE TABLE IF NOT EXISTS site_services (
    id         SERIAL PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    label      TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_site_services_site_id ON site_services(site_id);

CREATE TABLE IF NOT EXISTS site_certifications (
    id         SERIAL PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    label      TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_site_certifications_site_id ON site_certifications(site_id);

CREATE TABLE IF NOT EXISTS site_testimonials (
    id          SERIAL PRIMARY KEY,
    site_id     INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    author_name TEXT NOT NULL DEFAULT '',
    author_role TEXT NOT NULL DEFAULT '',
    quote       TEXT NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_site_testimonials_site_id ON site_testimonials(site_id);

CREATE TABLE IF NOT EXISTS site_gallery_images (
    id         SERIAL PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    url        TEXT NOT NULL,
    alt_text   TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_site_gallery_images_site_id ON site_gallery_images(site_id);

CREATE TABLE IF NOT EXISTS site_business_hours (
    id         SERIAL PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    label      TEXT NOT NULL,
    hours_text TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_site_business_hours_site_id ON site_business_hours(site_id);

CREATE TABLE IF NOT EXISTS leads (
    id         SERIAL PRIMARY KEY,
    site_id    INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    name       TEXT NOT NULL DEFAULT '',
    email      TEXT NOT NULL DEFAULT '',
    phone      TEXT NOT NULL DEFAULT '',
    message    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_leads_site_id ON leads(site_id);

CREATE TABLE IF NOT EXISTS page_views (
    id           SERIAL PRIMARY KEY,
    site_id      INTEGER NOT NULL REFERENCES sites(id) ON DELETE CASCADE,
    path         TEXT NOT NULL DEFAULT '',
    referrer     TEXT NOT NULL DEFAULT '',
    visitor_hash TEXT NOT NULL DEFAULT '', -- salted hash of IP, for approximate unique-visitor counts without storing raw IPs
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_page_views_site_id_created_at ON page_views(site_id, created_at);

-- Stripe webhook event IDs, for idempotent webhook processing.
CREATE TABLE IF NOT EXISTS stripe_events (
    event_id    TEXT PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
