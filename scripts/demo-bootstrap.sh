#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

docker compose up -d --build
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < infra/demo_seed.sql

until curl --fail --silent --show-error http://localhost:8080/health/ready >/dev/null; do
  sleep 2
done
until curl --fail --silent --show-error http://localhost:3000/api/health >/dev/null; do
  sleep 2
done

echo "RecoverOS is ready."
echo "Dashboard:     http://localhost:3000"
echo "Operations:    http://localhost:3000/operations"
echo "Observability: http://localhost:3000/observability"
echo "Demo replay:   http://localhost:3000/recovery/demo-recovered-case-v1"
