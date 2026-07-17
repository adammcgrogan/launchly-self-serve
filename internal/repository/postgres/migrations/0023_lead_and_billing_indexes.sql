DROP INDEX IF EXISTS idx_leads_site_id;
CREATE INDEX IF NOT EXISTS idx_leads_site_id_created_at ON leads(site_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_site_billing_trial_ends_at ON site_billing(trial_ends_at)
	WHERE payment_status NOT IN ('paid', 'cancelled');
