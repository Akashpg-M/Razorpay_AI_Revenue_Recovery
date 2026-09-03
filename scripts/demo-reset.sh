#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"
app_env=$(sed -n 's/^APP_ENV=//p' .env | tr -d '\r' | tail -n 1)
case "$app_env" in development|demo|test) ;; *) echo "Refusing reset: APP_ENV must be development, demo, or test." >&2; exit 1;; esac
if [ "${REQUIRE_DEMO_RESET:-}" != "YES" ]; then
  echo "Refusing reset: run REQUIRE_DEMO_RESET=YES sh scripts/demo-reset.sh" >&2
  exit 1
fi
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < infra/demo_reset.sql
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < infra/demo_seed.sql
echo "Dynamic frontend demo cases removed and canonical fixtures restored."
echo "Synthetic evaluation artifacts and provider-side Test Mode objects were not modified."
