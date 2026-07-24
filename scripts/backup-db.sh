#!/usr/bin/env bash
# Dumps the Launchly Postgres database (Supabase) to a single timestamped
# custom-format file, for supplemental backups outside of Supabase's own
# managed backups/PITR (see docs/backups.md).
#
# Usage:
#   DATABASE_URL=postgres://... ./scripts/backup-db.sh [output-dir]
#
# Requires the Postgres client tools (pg_dump) matching Supabase's Postgres
# major version — install via `brew install libpq` (macOS) or the
# `postgresql-client` package (Debian/Ubuntu), and ensure pg_dump is on PATH.
#
# Restore with:
#   pg_restore --clean --if-exists --no-owner --dbname "$DATABASE_URL" <file>

set -euo pipefail

if [ -z "${DATABASE_URL:-}" ]; then
  echo "error: DATABASE_URL is not set" >&2
  exit 1
fi

if ! command -v pg_dump >/dev/null 2>&1; then
  echo "error: pg_dump not found on PATH" >&2
  exit 1
fi

out_dir="${1:-./backups}"
mkdir -p "$out_dir"

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
out_file="$out_dir/launchly-${timestamp}.dump"

echo "Dumping database to $out_file ..."
pg_dump --format=custom --no-owner --no-acl --file="$out_file" "$DATABASE_URL"
echo "Done: $out_file"
