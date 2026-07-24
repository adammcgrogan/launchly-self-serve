# Postgres backup strategy

Launchly's database is Supabase Postgres (the app itself runs on Railway; `DATABASE_URL` points at Supabase — see `README.md`). Everything — site content, leads, billing state, accounts — lives in this one instance.

## What Supabase already does

Supabase manages backups itself, scaled by project plan:

- **Free plan** — no automated backups. Not appropriate for production data.
- **Pro plan** — automatic daily backups, retained 7 days.
- **Team plan** — daily backups retained 14 days, plus optional Point-in-Time Recovery (PITR) as an add-on (continuous WAL archiving, restore to any point within the retention window, typically 7–28 days depending on the add-on tier).

**Action needed (manual, one-time, in the Supabase dashboard):** confirm the project is on at least the Pro plan, and check *Project Settings → Backups* to see the current schedule/retention and whether PITR is enabled. If the project is still on the Free plan, upgrade it — this is the single highest-leverage step, more important than anything below.

### Restoring from a Supabase backup

Project Settings → Backups → pick a daily backup or a PITR timestamp → Restore. Supabase restores in place (it does not require a new project). Read the confirmation dialog carefully — this replaces the current database contents.

## Supplemental backup (this repo)

Supabase's backups are the primary safety net, but they're single-vendor and only reachable through the Supabase dashboard/support. As a cheap supplement, this repo includes:

- `scripts/backup-db.sh` — runs `pg_dump` against `DATABASE_URL` and writes a timestamped custom-format dump (`launchly-YYYYMMDDTHHMMSSZ.dump`). Run it manually any time:

  ```bash
  DATABASE_URL=postgres://... ./scripts/backup-db.sh ./backups
  ```

- `.github/workflows/backup.yml` — runs the script nightly (03:00 UTC) and uploads the dump as a GitHub Actions artifact, retained 30 days. This is **opt-in and does nothing until configured**:
  1. Add a `DATABASE_URL` repository secret (Settings → Secrets and variables → Actions → Secrets) — same connection string used in Railway.
  2. Add a `BACKUPS_ENABLED` repository variable (Settings → Secrets and variables → Actions → Variables) set to `true`.

  Until both are set, the workflow is a no-op (guarded by `if: vars.BACKUPS_ENABLED == 'true'`) so it doesn't fail loudly on every scheduled run.

GitHub Actions artifacts aren't a real backup vault (30-day retention, tied to repo access) — treat this as a second copy for disaster recovery, not a replacement for Supabase's own backups/PITR.

### Restoring from a supplemental dump

```bash
pg_restore --clean --if-exists --no-owner --dbname "$DATABASE_URL" launchly-<timestamp>.dump
```

Run this against a fresh/empty database when possible (e.g. a new Supabase project) rather than the live one, unless you've confirmed the current data should be discarded — `--clean` drops existing objects before recreating them.

## Retention summary

| Source | Frequency | Retention |
|---|---|---|
| Supabase managed backups | Daily (+ PITR if enabled) | 7–28 days, plan-dependent |
| `scripts/backup-db.sh` via GitHub Actions | Nightly | 30 days (GitHub artifact limit) |

## What's not covered

This backs up the Postgres database only. It does not cover Supabase Storage (uploaded logo/gallery images) or Stripe/Resend/Cloudflare configuration — those are managed by their respective vendors and aren't in scope for this issue.
