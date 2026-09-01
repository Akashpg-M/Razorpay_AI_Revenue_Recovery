# Observability

Go API and worker expose `/metrics`; FastAPI exposes `/metrics`. Correlation IDs are accepted/generated at the API boundary, returned in response headers, propagated to the decision service, and included in structured JSON request logs. Route labels use templates to avoid case-ID cardinality.

`/observability` reads durable queue, execution, case, and promise truth. `/health/live` proves the process is running; `/health/ready` requires PostgreSQL, the decision service where applicable, and schema `phase_34`. Redis is reported as optional. Development alerts flag queue lag, execution failures, and overdue promises.
