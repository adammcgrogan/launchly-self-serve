# Launchly

Self-serve website builder for local businesses. Sign up, build a site, and it's live immediately — no approval, no manual publish step, no admin-sent payment links.

## Stack

- **Go** — single binary, standard library HTTP server
- **Supabase** — Postgres hosting + Auth (email/password, email verification, password reset)
- **html/template + Tailwind CSS** — server-rendered, no build step
- **Stripe** — self-serve subscription billing (Starter £14/mo, Pro £34/mo)
- **Resend** — transactional email (welcome, leads, billing, trial reminders, analytics digests)
- **Railway** — hosting

## How it works

1. A business owner signs up with email + password (Supabase Auth) — no card required.
2. They pick a design and fill in their content in the builder wizard.
3. The site publishes immediately at `slug.launchly.ltd` — no review step.
4. A 14-day free trial starts automatically; upgrading is a button in the dashboard, not an email from Adam.
5. Visitors submit the contact form — the lead is saved and emailed to the business.
6. The business can edit content, appearance, or switch designs any time from their dashboard.

A read-mostly superadmin view (`/superadmin`, shared password) exists for cross-account visibility and an emergency unpublish/delete backstop — nothing in the customer-facing flow depends on it.

## Deployment

Runs on Railway. Set these as env vars in the Railway dashboard (see `internal/config` for the full list): `DATABASE_URL`, `DOMAIN`, `SUPABASE_URL`, `SUPABASE_ANON_KEY`, `SUPABASE_SERVICE_ROLE_KEY`, `SUPABASE_JWT_SECRET`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_STARTER_PRICE_ID`, `STRIPE_PRO_PRICE_ID`, `RESEND_API_KEY`, `EMAIL_FROM`, `SUPERADMIN_PASSWORD`, `COOKIE_SIGNING_KEY`.

Migrations apply automatically on startup — there's no separate migration command to run.

## Project structure

```
cmd/server/main.go        — entry point: config, wiring, routing, middleware, graceful shutdown
internal/
  config/                  — typed env var loading
  domain/                  — pure types shared across layers (Site, Profile, Lead, Template, ...)
  supabase/                — Supabase Auth REST client + local JWT verification
  repository/postgres/     — data access only, one file per aggregate; migrations/ holds versioned .sql
  service/                 — business logic: accounts, sites, billing, leads, analytics, cron
  email/                   — Resend client + templated app emails
  payment/                 — Stripe Checkout + webhook handling
  web/                     — HTTP layer: handlers, router, template renderer
    middleware/            — Supabase auth, superadmin auth, site ownership, CSRF, rate limiting, flash
web/
  templates/
    public/                 — marketing pages
    auth/                    — signup, login, password reset
    dashboard/               — multi-site management, builder, editor
    superadmin/
    sites/                   — the site designs themselves (aurora, foundry, meridian, bloom)
  static/
```

Each layer only calls the layer below it: `web` → `service` → `repository`/`supabase`/`email`/`payment`. Business rules live in `service/`, not scattered across handlers.
