-- Lets Pro sites connect their own domain, proxied through Cloudflare for
-- SaaS (see internal/cloudflare) so TLS and per-tenant routing don't depend
-- on Railway's own custom-domain limits.
ALTER TABLE sites ADD COLUMN IF NOT EXISTS custom_domain TEXT UNIQUE;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS custom_domain_status TEXT NOT NULL DEFAULT 'none';
ALTER TABLE sites ADD COLUMN IF NOT EXISTS custom_domain_cf_id TEXT;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS custom_domain_added_at TIMESTAMPTZ;
