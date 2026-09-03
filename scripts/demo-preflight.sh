#!/usr/bin/env sh
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT"

command -v docker >/dev/null
command -v curl >/dev/null
docker compose config --quiet

wait_url() {
  url=$1
  attempts=0
  until curl --fail --silent --show-error "$url" >/dev/null 2>&1; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 30 ]; then echo "FAIL: timed out waiting for $url" >&2; exit 1; fi
    sleep 2
  done
}

wait_url http://localhost:8080/health/ready
wait_url http://localhost:8001/health/ready
wait_url http://localhost:3000/api/health

backend=$(curl --fail --silent --show-error http://localhost:8080/health/ready)
echo "$backend" | grep -q '"status":"ready"'
echo "$backend" | grep -q '"schema":"phase_55"'

provider=$(curl --fail --silent --show-error http://localhost:8080/api/v1/integrations/razorpay/status)
echo "$provider" | grep -q '"selected_provider":"razorpay"'
echo "$provider" | grep -q '"authenticated":true'
echo "$provider" | grep -q '"webhook_verification_configured":true'
echo "$provider" | grep -q '"external_webhook_delivery_configured":true'

webhook_url=$(sed -n 's/^RAZORPAY_WEBHOOK_PUBLIC_URL=//p' .env | tr -d '\r' | tail -n 1)
test -n "$webhook_url"
public_base=${webhook_url%/api/v1/webhooks/razorpay}
curl --fail --silent --show-error "$public_base/health/ready" >/dev/null

# Git Bash rewrites POSIX-looking arguments unless path conversion is disabled.
MSYS_NO_PATHCONV=1 docker compose run --rm --no-deps backend /app/razorpay-check >/dev/null

echo "PASS: RecoverOS phase_55 is ready."
echo "PASS: Razorpay Test Mode is authenticated and webhook HMAC is configured."
echo "PASS: Public tunnel reaches backend readiness: $public_base"
echo "OPEN: http://localhost:3000/demo"
