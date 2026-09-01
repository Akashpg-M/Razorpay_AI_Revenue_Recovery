#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT/backend"
test -z "$(gofmt -l .)"
go vet ./...
go test ./...
go build ./cmd/api ./cmd/worker ./cmd/evaluation
cd "$ROOT/decision-service"
python -m unittest discover -s tests -p 'test_*.py'
cd "$ROOT/frontend"
npm run lint
npm run build
mkdir -p "$ROOT/evaluation/results/phase33"
printf '{"schema_version":"verification-summary-v1","status":"PASS","checks":{"go":"PASS","python":"PASS","frontend":"PASS"}}\n' > "$ROOT/evaluation/results/phase33/verification_summary.json"
