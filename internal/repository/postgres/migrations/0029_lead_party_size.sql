-- Adds the "appointment" form type (service picker + preferred date + party
-- size), a more detailed alternative to the plain booking form.
ALTER TABLE leads ADD COLUMN IF NOT EXISTS party_size TEXT NOT NULL DEFAULT '';
