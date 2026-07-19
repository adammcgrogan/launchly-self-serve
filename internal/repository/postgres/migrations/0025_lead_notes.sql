-- Per-lead notes: lets an owner jot follow-up context and call-back
-- reminders against an enquiry, turning the leads table into a lightweight
-- CRM instead of just a status pill.
CREATE TABLE IF NOT EXISTS lead_notes (
    id         SERIAL PRIMARY KEY,
    lead_id    INTEGER NOT NULL REFERENCES leads(id) ON DELETE CASCADE,
    body       TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_lead_notes_lead_id ON lead_notes(lead_id);
