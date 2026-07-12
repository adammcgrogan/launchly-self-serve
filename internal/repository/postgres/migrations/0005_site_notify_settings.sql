-- Lets Pro owners opt into an SMS text alert (on top of the existing email
-- notification) whenever their site gets a new lead.
CREATE TABLE IF NOT EXISTS site_notify_settings (
    site_id            INTEGER PRIMARY KEY REFERENCES sites(id) ON DELETE CASCADE,
    mobile_number      TEXT NOT NULL DEFAULT '',
    sms_alerts_enabled BOOLEAN NOT NULL DEFAULT false
);
