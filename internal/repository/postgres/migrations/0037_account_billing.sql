-- Move plan/subscription state from per-site to per-account (#338).
--
-- Pro was sold as "unlimited sites" but only lifted the creation cap: every
-- additional site got its own site_billing row with its own 7-day trial and
-- its own subscription requirement, so extra sites went dark on day 7 even
-- though the account was paying. Billing now hangs off the account, and
-- every site the account owns inherits it.
CREATE TABLE IF NOT EXISTS account_billing (
    owner_user_id                 UUID PRIMARY KEY REFERENCES profiles(id) ON DELETE CASCADE,
    plan                          TEXT NOT NULL DEFAULT 'starter',
    payment_status                TEXT NOT NULL DEFAULT 'trialing',
    stripe_customer_id            TEXT NOT NULL DEFAULT '',
    stripe_session_id             TEXT NOT NULL DEFAULT '',
    stripe_subscription_id        TEXT NOT NULL DEFAULT '',
    paid_at                       TIMESTAMPTZ,
    trial_ends_at                 TIMESTAMPTZ,
    trial_reminder_sent_at        TIMESTAMPTZ,
    trial_final_reminder_sent_at  TIMESTAMPTZ,
    trial_early_report_sent_at    TIMESTAMPTZ,
    trial_report_sent_at          TIMESTAMPTZ,
    payment_failed_at             TIMESTAMPTZ,
    dunning_reminder_1_sent_at    TIMESTAMPTZ,
    dunning_reminder_2_sent_at    TIMESTAMPTZ,
    dunning_final_warning_sent_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_account_billing_stripe_session_id
    ON account_billing (stripe_session_id) WHERE stripe_session_id != '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_account_billing_stripe_subscription_id
    ON account_billing (stripe_subscription_id) WHERE stripe_subscription_id != '';

-- Fold each account's existing per-site rows into one. Ordering picks the
-- most-committed row an account has (paid over pending over trialing, then
-- the longest-running trial), so nobody is downgraded by the collapse.
INSERT INTO account_billing (
    owner_user_id, plan, payment_status, stripe_customer_id, stripe_session_id,
    stripe_subscription_id, paid_at, trial_ends_at, trial_reminder_sent_at,
    trial_final_reminder_sent_at, trial_early_report_sent_at, trial_report_sent_at,
    payment_failed_at, dunning_reminder_1_sent_at, dunning_reminder_2_sent_at,
    dunning_final_warning_sent_at
)
SELECT DISTINCT ON (s.owner_user_id)
    s.owner_user_id, sb.plan, sb.payment_status, sb.stripe_customer_id, sb.stripe_session_id,
    sb.stripe_subscription_id, sb.paid_at, sb.trial_ends_at, sb.trial_reminder_sent_at,
    sb.trial_final_reminder_sent_at, sb.trial_early_report_sent_at, sb.trial_report_sent_at,
    sb.payment_failed_at, sb.dunning_reminder_1_sent_at, sb.dunning_reminder_2_sent_at,
    sb.dunning_final_warning_sent_at
FROM site_billing sb
JOIN sites s ON s.id = sb.site_id
ORDER BY s.owner_user_id,
         CASE sb.payment_status WHEN 'paid' THEN 0 WHEN 'past_due' THEN 1
                                WHEN 'pending' THEN 2 WHEN 'trialing' THEN 3 ELSE 4 END,
         CASE sb.plan WHEN 'pro' THEN 0 ELSE 1 END,
         sb.trial_ends_at DESC NULLS LAST
ON CONFLICT (owner_user_id) DO NOTHING;

DROP TABLE IF EXISTS site_billing;
