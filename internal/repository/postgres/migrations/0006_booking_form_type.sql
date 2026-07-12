-- Lets an owner switch their site's public form from a plain contact form
-- to a booking/appointment form (service picker + preferred time), and
-- records those two extra fields on leads submitted through it.
ALTER TABLE sites ADD COLUMN IF NOT EXISTS form_type TEXT NOT NULL DEFAULT 'contact';
ALTER TABLE leads ADD COLUMN IF NOT EXISTS service_label TEXT NOT NULL DEFAULT '';
ALTER TABLE leads ADD COLUMN IF NOT EXISTS preferred_time TEXT NOT NULL DEFAULT '';
