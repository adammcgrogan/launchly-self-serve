-- Dunning sequence for failed payments (#59). Before this, a payment failure
-- sent a single email and then relied entirely on Stripe's own retry
-- schedule, with no follow-up and no distinct state until the subscription
-- was eventually cancelled. These columns let a cron sweep track a failed
-- payment's own timeline (independent of how many times Stripe retries the
-- charge) so it can escalate reminder emails and, if the customer never
-- fixes their payment method, cancel the subscription itself rather than
-- leaving the site paid-but-broken indefinitely.
ALTER TABLE site_billing ADD COLUMN IF NOT EXISTS payment_failed_at TIMESTAMPTZ;
ALTER TABLE site_billing ADD COLUMN IF NOT EXISTS dunning_reminder_1_sent_at TIMESTAMPTZ;
ALTER TABLE site_billing ADD COLUMN IF NOT EXISTS dunning_reminder_2_sent_at TIMESTAMPTZ;
ALTER TABLE site_billing ADD COLUMN IF NOT EXISTS dunning_final_warning_sent_at TIMESTAMPTZ;
