-- The analytics digest email is now monthly-only (no more weekly cadence) —
-- migrate existing weekly subscribers to monthly instead of silently
-- dropping their subscription.
UPDATE site_analytics_settings SET analytics_frequency = 'monthly' WHERE analytics_frequency = 'weekly';
