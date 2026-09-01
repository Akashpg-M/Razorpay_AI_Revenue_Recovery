# What broke

Historical regression targets include stale decisions executing after a case mutation, worker comparisons against the wrong decision-era version, duplicate provider work under retries, hidden simulator feature leakage, natural recovery being credited to the agent, promise checks surviving terminal outcomes, and human approvals being treated as unconditional overrides.

The current design fixes these with schedule-era versions, execution-time policy checks, stable idempotency, observable-only feature contracts, explicit attribution classes, durable promise cancellation, and transactional approval reauthorization. The top-level verification workflow is the guard against recurrence.
