# RecoverOS startup guide — Git Bash

This is the complete from-scratch startup path for Windows using Git Bash and Docker Desktop. Run each command from Git Bash, not PowerShell or Command Prompt.

## 1. Install the host prerequisites

Install:

1. Git for Windows, including Git Bash.
2. Docker Desktop with Linux containers and Docker Compose v2.
3. At least 8 GB RAM available to Docker and approximately 10 GB free disk space.

Go, Python, Node.js, PostgreSQL, Redis, and the migration CLI do **not** need to be installed on the host for the Docker startup path. Docker supplies:

- Go 1.25 for backend builds;
- Python 3.12 and the pinned packages in `decision-service/requirements.txt`;
- Node.js 22 and the locked packages in `frontend/package-lock.json`;
- PostgreSQL 16;
- Redis 7;
- `migrate/migrate` 4.18.3.

Open Docker Desktop and wait until it reports that the engine is running. Then verify the tools:

```bash
git --version
docker --version
docker compose version
docker info
```

All four commands must succeed before continuing.

## 2. Open the repository

For the current checkout:

```bash
cd /c/Users/akash/PROJECTS/Razorpay/revenue-recovery
pwd
```

`pwd` should end in `/Razorpay/revenue-recovery`.

For a new clone on another machine, use the real repository URL in place of the placeholder:

```bash
git clone REPLACE_WITH_REPOSITORY_URL revenue-recovery
cd revenue-recovery
```

## 3. Create the local environment file

Create `.env` without overwriting an existing one:

```bash
test -f .env || cp .env.example .env
```

Confirm that the required development settings are present (this command does
not print secret values):

```bash
awk -F= '/^(APP_ENV|FRONTEND_PORT|BACKEND_PORT|DECISION_SERVICE_PORT|POSTGRES_DB|POSTGRES_USER|POSTGRES_PORT|REDIS_PORT)=/ {print $1 "=SET"}' .env
```

The default ports are:

| Component | Port |
|---|---:|
| Frontend | 3000 |
| Backend API | 8080 |
| Decision service | 8001 |
| PostgreSQL | 5432 |
| Redis | 6379 |

Razorpay credentials may remain blank for local development, synthetic evaluation, local email simulation, retry-outbox behavior, dashboards, replay, and human-review testing. To use Razorpay Test Mode, edit `.env`:

```bash
notepad .env
```

Set only Test Mode values:

```text
RAZORPAY_KEY_ID=your_test_key_id
RAZORPAY_KEY_SECRET=your_test_key_secret
RAZORPAY_WEBHOOK_SECRET=your_test_webhook_secret
RAZORPAY_API_URL=https://api.razorpay.com
PAYMENT_PROVIDER=local
```

Keep `PAYMENT_PROVIDER=local` for deterministic development and CI. Change it
to `razorpay` only when you intentionally want the worker to create Razorpay
Test Mode Payment Links. Live keys are rejected by the integration. Never
commit `.env` or paste its secrets into logs.

## 4. Validate and start the complete stack

Validate the resolved Compose configuration:

```bash
docker compose config --quiet
```

Build and start every service:

```bash
docker compose up -d --build
```

This starts PostgreSQL, Redis, the migration job, the Python decision service, the Go backend, the durable Go worker, and the Next.js frontend. Database migrations run automatically before the backend and worker become ready.

Wait for backend readiness:

```bash
until curl --fail --silent http://localhost:8080/health/ready >/dev/null; do sleep 2; done
```

Wait for the decision service:

```bash
until curl --fail --silent http://localhost:8001/health/ready >/dev/null; do sleep 2; done
```

Wait for the frontend:

```bash
until curl --fail --silent http://localhost:3000/api/health >/dev/null; do sleep 2; done
```

Inspect all containers:

```bash
docker compose ps
```

Expected long-running services are `postgres`, `redis`, `decision-service`, `backend`, `worker`, and `frontend`. The `migrate` service should show `Exited (0)`; that means migrations completed successfully.

Run the judge-demo preflight after the services become healthy:

```bash
sh scripts/demo-preflight.sh
```

The expected readiness schema is `phase_55`.

## 5. Load the deterministic demo data

The demo dataset is Test Mode-only, contains no real contact data, and is safe to run repeatedly:

```bash
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < infra/demo_seed.sql
```

Alternatively, startup and seeding can be performed together:

```bash
sh scripts/demo-bootstrap.sh
```

The seed creates:

- a recovered case for complete replay;
- an active promise-to-pay case;
- a high-value escalated case in the human operations queue.

## 6. Verify every service

Backend readiness and schema version:

```bash
curl --fail --silent http://localhost:8080/health/ready
echo
```

Decision-service readiness:

```bash
curl --fail --silent http://localhost:8001/health/ready
echo
```

Frontend health:

```bash
curl --fail --silent http://localhost:3000/api/health
echo
```

Operational health snapshot:

```bash
curl --fail --silent http://localhost:8080/api/v1/observability
echo
```

Human-review queue:

```bash
curl --fail --silent http://localhost:8080/api/v1/operations/recovery-queue
echo
```

Recovered-case replay:

```bash
curl --fail --silent http://localhost:8080/api/v1/recovery-cases/demo-recovered-case-v1/replay
echo
```

Metrics endpoints:

```bash
curl --fail --silent http://localhost:8080/metrics
curl --fail --silent http://localhost:8001/metrics
```

## 6a. Verify optional Razorpay Test Mode integration

The normal stack does not require Razorpay. With Test Mode credentials in
`.env`, inspect the development-safe status endpoint:

```bash
curl --fail --silent http://localhost:8080/api/v1/integrations/razorpay/status
echo
```

It reports only booleans, mode, non-secret API URL, HTTP status, and supported
capabilities. It never returns credentials or authorization headers.

Seed the deterministic fixtures before using the verification CLI:

```bash
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < infra/demo_seed.sql
```

Run a harmless authenticated Razorpay GET:

```bash
docker compose exec -T backend ./razorpay-check
```

Optionally create one synthetic ₹1 Test Mode Payment Link, persist its provider
reference, and fetch it back:

```bash
docker compose exec -T backend ./razorpay-check --create-payment-link
```

The operation has a stable reference ID, so rerunning it reuses the persisted
provider reference instead of creating another Payment Link. Razorpay limits
the number of Test Mode links, so do not change the reference merely to repeat
the test.

The relative `./razorpay-check` path is intentional. In Git Bash, passing the
absolute Linux path `/app/razorpay-check` to `docker compose run` can be rewritten
to `C:/Program Files/Git/app/razorpay-check`, which does not exist inside the
Linux container. `compose exec` also reuses the healthy backend container and
its established network instead of creating a short-lived container.

To use Razorpay for worker-executed `SEND_PAYMENT_LINK` actions, edit `.env`:

```text
PAYMENT_PROVIDER=razorpay
```

Then recreate only the affected services:

```bash
docker compose up -d --build --force-recreate backend worker
```

To return to deterministic local execution, set `PAYMENT_PROVIDER=local` and
run the same recreate command.

### Configure Test Mode webhook delivery

The inbound route is:

```text
POST https://YOUR_PUBLIC_TEST_HOST/api/v1/webhooks/razorpay
```

`localhost` is not reachable by Razorpay. Expose the backend through a secure
public test/staging URL, then configure that exact URL in the Razorpay Dashboard
while the dashboard is in Test Mode. Use exactly the same webhook secret in the
Dashboard and `.env`. For the currently verified account select
`payment.failed` and `payment_link.paid`; its event picker does not expose the
subscription/mandate events that the normalizer also recognizes. Set:

```text
RAZORPAY_WEBHOOK_PUBLIC_URL=https://YOUR_PUBLIC_TEST_HOST/api/v1/webhooks/razorpay
```

Recreate the backend after changing it:

```bash
docker compose up -d --build --force-recreate backend
```

This variable records configuration intent for the status endpoint; a real
Test Mode event must still be triggered to prove external delivery. The backend
validates `X-Razorpay-Signature` against the raw body and deduplicates by
`X-Razorpay-Event-Id`.

For an interstitial-free temporary Cloudflare quick tunnel, install
`cloudflared`, keep this command running in a separate Git Bash window, and
copy the generated `https://...trycloudflare.com` hostname:

```bash
cloudflared tunnel --url http://localhost:8080
```

Set the Dashboard webhook and `.env` to:

```text
RAZORPAY_WEBHOOK_PUBLIC_URL=https://GENERATED.trycloudflare.com/api/v1/webhooks/razorpay
```

Verify the exact tunnel before the demo:

```bash
curl --fail "https://GENERATED.trycloudflare.com/health/ready"
```

Quick-tunnel hostnames are temporary. If the tunnel restarts, update both the
Razorpay Test Mode dashboard and `.env`, then recreate the backend.

If using a free `*.shares.zrok.io` endpoint, test it without browser cookies:

```bash
curl -i "$RAZORPAY_WEBHOOK_PUBLIC_URL"
```

The current share returns a zrok interstitial unless callers add
`skip_zrok_interstitial`. Razorpay cannot be assumed to add this custom header,
so that URL is suitable for manual tunnel tests only. For Dashboard delivery,
use an interstitial-free zrok share/domain or another HTTPS tunnel. A successful
`/health/ready` request made with the bypass header does not prove Razorpay can
reach the webhook route.

## 7. Open the application

Open these URLs in a browser:

```text
http://localhost:3000
http://localhost:3000/demo
http://localhost:3000/recoveries
http://localhost:3000/operations
http://localhost:3000/evaluation
http://localhost:3000/observability
http://localhost:3000/resilience
http://localhost:3000/recovery/demo-recovered-case-v1
```

From Git Bash on Windows, the first page can be opened with:

```bash
cmd.exe /c start "" "http://localhost:3000"
```

## 8. View logs and diagnose startup failures

Follow all application logs:

```bash
docker compose logs --follow --tail=200 backend worker decision-service frontend
```

View a single component:

```bash
docker compose logs --follow --tail=200 backend
docker compose logs --follow --tail=200 worker
docker compose logs --follow --tail=200 decision-service
docker compose logs --follow --tail=200 frontend
docker compose logs --tail=200 migrate
docker compose logs --tail=200 postgres
```

Press `Ctrl+C` to stop following logs; this does not stop the containers.

Check whether a required port is already occupied:

```bash
netstat -ano | grep -E ':(3000|8001|8080|5432|6379)[[:space:]]'
```

If a port is occupied, change its host-side value in `.env`, then recreate the stack:

```bash
docker compose up -d --build --force-recreate
```

If a build was interrupted, retry it:

```bash
docker compose build
docker compose up -d
```

## 9. Run the complete verification suite

The full Git Bash-native verification script requires local Go 1.25, Python 3.12 with `requirements.txt`, and Node.js 22 with npm:

```bash
sh scripts/verify.sh
```

On this Windows project, the recommended pinned-environment verification uses Docker through the checked-in PowerShell runner. It can still be launched from Git Bash:

```bash
powershell.exe -ExecutionPolicy Bypass -File ./scripts/verify.ps1
```

Inspect the machine-readable result:

```bash
cat evaluation/results/phase33/verification_summary.json
```

The top-level status must be `PASS`.

## 10. Normal restart and shutdown commands

Restart all services without deleting data:

```bash
docker compose restart
```

Restart only an application service:

```bash
docker compose restart backend
docker compose restart worker
docker compose restart decision-service
docker compose restart frontend
```

Stop and remove containers while preserving PostgreSQL and Redis volumes:

```bash
docker compose down
```

Start them again with the preserved data:

```bash
docker compose up -d
```

## 11. Completely reset local data

Warning: the next command permanently deletes this project's local PostgreSQL and Redis Docker volumes. Use it only when a completely clean database is intended:

```bash
docker compose down --volumes --remove-orphans
```

Rebuild the clean stack and seed it again:

```bash
docker compose up -d --build
until curl --fail --silent http://localhost:8080/health/ready >/dev/null; do sleep 2; done
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < infra/demo_seed.sql
```

## 12. Optional native-development dependencies

Docker is the supported complete startup path. If running processes directly on the host, install the exact major versions below:

```text
Go 1.25+
Python 3.12
Node.js 22 and npm
PostgreSQL 16
Redis 7
golang-migrate 4.18+
```

Install Python and frontend packages:

```bash
cd decision-service
python -m venv .venv
source .venv/Scripts/activate
python -m pip install --upgrade pip
python -m pip install -r requirements.txt
cd ../frontend
npm ci
cd ..
```

Download Go modules:

```bash
cd backend
go mod download
cd ..
```

For native development, PostgreSQL migrations must be applied and `DATABASE_URL`, `REDIS_URL`, and `DECISION_SERVICE_URL` must match the native services. Prefer Docker Compose unless debugging one component specifically, because it provides the tested dependency versions and startup ordering automatically.

## Quick-start command block

For subsequent clean installations where Git, Docker Desktop, and the repository already exist, these are the only commands required:

```bash
cd /c/Users/akash/PROJECTS/Razorpay/revenue-recovery
test -f .env || cp .env.example .env
docker compose config --quiet
docker compose up -d --build
until curl --fail --silent http://localhost:8080/health/ready >/dev/null; do sleep 2; done
until curl --fail --silent http://localhost:8001/health/ready >/dev/null; do sleep 2; done
until curl --fail --silent http://localhost:3000/api/health >/dev/null; do sleep 2; done
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < infra/demo_seed.sql
docker compose ps
cmd.exe /c start "" "http://localhost:3000"
```
