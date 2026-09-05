-- Trial-value report emails (#330). Two sent-markers mirroring the existing
-- trial_reminder_sent_at / trial_final_reminder_sent_at pair, so each report
-- is marked and skipped independently and a cron gap can't double-send one.
ALTER TABLE site_billing ADD COLUMN IF NOT EXISTS trial_early_report_sent_at TIMESTAMPTZ;
ALTER TABLE site_billing ADD COLUMN IF NOT EXISTS trial_report_sent_at TIMESTAMPTZ;
