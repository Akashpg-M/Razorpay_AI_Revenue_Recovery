# RevClaim Setup Guide

This is the supported from-scratch setup for **Windows + Git Bash + Docker Desktop**. Docker Compose supplies Go, Node.js, Python, PostgreSQL, Redis, and the migration tool; you do not need to install those runtimes directly.

## Prerequisites

- Git for Windows, including Git Bash
- Docker Desktop using Linux containers and Docker Compose v2
- `curl` (included with current Git for Windows installations)
- Optional: `cloudflared` for Razorpay webhook testing
- Optional: a Razorpay Test Mode account and Test Mode keys

Start Docker Desktop and wait for the engine to become ready. In **Git Bash**, verify:

```bash
git --version
docker --version
docker compose version
docker info
```

## 1. Clone the repository

```bash
git clone <repository-url>
cd revenue-recovery
```

All remaining commands must be run from the repository root.

## 2. Configure the environment

Create the local environment file:

```bash
cp .env.example .env
```

`.env` is ignored by Git. Never commit it or publish its secret values.

The checked-in defaults run the full local application with `PAYMENT_PROVIDER=local`. The available variables are:

| Variable | Purpose | Default/example behavior |
|---|---|---|
| `APP_ENV` | Enables development/demo-only routes in an allowed non-production environment | `development` |
| `FRONTEND_PORT` | Host port for Next.js | `3000` |
| `BACKEND_PORT` | Host port for the Go API | `8080` |
| `FRONTEND_ORIGIN` | Allowed browser origin for backend CORS | `http://localhost:3000` |
| `WORKER_HEALTH_PORT` | Native worker health setting; Compose currently fixes the worker health port at `8082` | Not published to the host |
| `DECISION_SERVICE_PORT` | Host port for FastAPI | `8001` |
| `POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_PORT` | Local PostgreSQL database and host mapping | Development values are provided |
| `REDIS_PORT` | Local Redis host mapping | `6379` |
| `DATABASE_URL`, `REDIS_URL`, `DECISION_SERVICE_URL` | Native-development URLs; Compose injects its own service-network URLs | Localhost defaults are provided |
| `NEXT_PUBLIC_BACKEND_URL` | Backend URL used by the browser | `http://localhost:8080` |
| `PAYMENT_PROVIDER` | Worker executor: deterministic `local` or Razorpay Test Mode `razorpay` | `local` |
| `RAZORPAY_KEY_ID`, `RAZORPAY_KEY_SECRET` | Razorpay Test Mode API credentials | Empty |
| `RAZORPAY_WEBHOOK_SECRET` | Separate secret used to validate incoming webhook HMAC signatures | Empty |
| `RAZORPAY_WEBHOOK_PUBLIC_URL` | Non-secret marker for the configured public webhook endpoint | Empty |
| `RAZORPAY_API_URL` | Razorpay API base URL | `https://api.razorpay.com` |

Local mode does not require Razorpay credentials. To use Razorpay, edit `.env` and set only Test Mode credentials:

```text
PAYMENT_PROVIDER=razorpay
RAZORPAY_KEY_ID=your_test_key_id
RAZORPAY_KEY_SECRET=your_test_key_secret
RAZORPAY_WEBHOOK_SECRET=your_test_webhook_secret
RAZORPAY_WEBHOOK_PUBLIC_URL=https://YOUR_TEMPORARY_HOST/api/v1/webhooks/razorpay
```

The webhook secret is separate from the API key secret. Live-key prefixes are rejected by the integration.

## 3. Validate Docker Compose

```bash
docker compose config --quiet
```

No output and exit code `0` means the Compose configuration is valid.

## 4. Build and start

```bash
docker compose up -d --build
docker compose ps
```

Compose starts `postgres`, `redis`, `decision-service`, `backend`, `worker`, and `frontend`. The one-time `migrate` service should finish with `Exited (0)`; the long-running services should become healthy.

Wait for the three public application surfaces:

```bash
until curl --fail --silent http://localhost:8080/health/ready >/dev/null; do sleep 2; done
until curl --fail --silent http://localhost:8001/health/ready >/dev/null; do sleep 2; done
until curl --fail --silent http://localhost:3000/api/health >/dev/null; do sleep 2; done
```

Load the repeatable, non-production demo fixtures:

```bash
docker compose exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' < infra/demo_seed.sql
```

The seed includes a recovered replay case, an active Promise-to-Pay case, and a high-value case for human review. It contains no real customer contact data.

For a later clean startup, the checked-in helper combines stack startup and seeding:

```bash
sh scripts/demo-bootstrap.sh
```

## 5. Validate the running application

Basic local validation works in either provider mode:

```bash
curl --fail http://localhost:8080/health/ready
curl --fail http://localhost:8001/health/ready
curl --fail http://localhost:3000/api/health
curl --fail http://localhost:8080/api/v1/observability
```

Backend readiness should report schema `phase_55`. PostgreSQL and the decision service are authoritative readiness dependencies. Redis may report `optional_unavailable` at runtime without making the backend unready, although a cold Compose startup waits for the Redis container health check.

### Razorpay demo preflight

Run the full preflight only after configuring Razorpay Test Mode and the public webhook tunnel:

```bash
sh scripts/demo-preflight.sh
```

It verifies Compose configuration, frontend/backend/decision-service readiness, schema `phase_55`, selected Razorpay provider, Test Mode authentication, webhook-HMAC configuration, the configured public tunnel, and a harmless authenticated Razorpay API check.

To check Razorpay API authentication without creating a Payment Link:

```bash
docker compose exec -T backend ./razorpay-check
```

The relative executable path is intentional and avoids Git Bash rewriting `/app/...` into a Windows path.

## 6. Application URLs

| Surface | URL |
|---|---|
| RevClaim Command Center | <http://localhost:3000> |
| Guided demo | <http://localhost:3000/demo> |
| Recoveries | <http://localhost:3000/recoveries> |
| Human operations queue | <http://localhost:3000/operations> |
| Synthetic evaluation | <http://localhost:3000/evaluation> |
| Reliability Lab | <http://localhost:3000/resilience> |
| Observability | <http://localhost:3000/observability> |
| Go backend readiness | <http://localhost:8080/health/ready> |
| Go backend metrics | <http://localhost:8080/metrics> |
| Decision-service readiness | <http://localhost:8001/health/ready> |
| Decision-service metrics | <http://localhost:8001/metrics> |

Open the application from Git Bash:

```bash
cmd.exe /c start "" "http://localhost:3000"
```

## 7. Razorpay Test Mode and Cloudflare webhook setup

Skip this section when using `PAYMENT_PROVIDER=local`.

1. Start an interstitial-free Cloudflare Quick Tunnel in a second Git Bash window and keep it running:

   ```bash
   cloudflared tunnel --url http://localhost:8080
   ```

2. Copy the generated temporary `https://...trycloudflare.com` hostname. Configure this exact endpoint in the **Razorpay Test Mode** Dashboard:

   ```text
   https://YOUR_GENERATED_HOST/api/v1/webhooks/razorpay
   ```

3. Select the currently supported demo events `payment.failed` and `payment_link.paid`. Set the same webhook secret in the Razorpay Dashboard and `RAZORPAY_WEBHOOK_SECRET`.

4. Update `.env`:

   ```text
   PAYMENT_PROVIDER=razorpay
   RAZORPAY_WEBHOOK_PUBLIC_URL=https://YOUR_GENERATED_HOST/api/v1/webhooks/razorpay
   ```

5. Recreate the services that consume this configuration:

   ```bash
   docker compose up -d --build --force-recreate backend worker
   ```

6. Verify the exact public hostname and rerun the Razorpay preflight:

   ```bash
   curl --fail "https://YOUR_GENERATED_HOST/health/ready"
   sh scripts/demo-preflight.sh
   ```

Quick Tunnel hostnames are temporary. If the tunnel restarts, update both the Razorpay Test Mode Dashboard and `.env`, recreate `backend` and `worker`, and rerun preflight. A configured URL marker alone is not proof that Razorpay can deliver to it; verify a real Test Mode event in the Razorpay delivery log.

## 8. Logs and troubleshooting

Follow the main application logs:

```bash
docker compose logs -f --tail=200 backend worker decision-service frontend
```

Inspect one service or the migration job:

```bash
docker compose logs -f backend
docker compose logs -f worker
docker compose logs -f decision-service
docker compose logs --tail=200 migrate
```

Common issues:

- **Docker commands fail:** start Docker Desktop and wait for `docker info` to succeed.
- **A service cannot bind its port:** change the relevant host port in `.env`, then recreate the stack. Update URLs if you change frontend, backend, or decision-service ports.
- **Razorpay authentication fails:** confirm Test Mode is selected, the key pair belongs together, and `PAYMENT_PROVIDER=razorpay`; recreate `backend` and `worker` after changing `.env`.
- **Webhook preflight fails:** keep `cloudflared` running, update the temporary hostname in both Razorpay and `.env`, and recreate the backend.
- **Backend is not ready:** inspect `postgres`, `migrate`, `decision-service`, and backend logs; readiness requires schema `phase_55` and a reachable decision service.

## 9. Stop and restart

Stop containers while preserving PostgreSQL and Redis volumes:

```bash
docker compose down
```

Start again with preserved data:

```bash
docker compose up -d
```

Restart all running services without removing them:

```bash
docker compose restart
```

To return from Razorpay to deterministic local execution, set `PAYMENT_PROVIDER=local` in `.env`, then run:

```bash
docker compose up -d --build --force-recreate backend worker
```
