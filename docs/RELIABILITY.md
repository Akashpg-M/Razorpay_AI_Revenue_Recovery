# Reliability

PostgreSQL claim leases and `SKIP LOCKED` support concurrent at-least-once workers. Stable action/provider keys suppress duplicate side effects. Scheduling stores the authoritative case version; any later mutation supersedes stale work. Retries are bounded and outcomes are observed separately from dispatch.

Run `scripts/verify-live.ps1` for live readiness and `make verify` or `scripts/verify.ps1` for local regression verification. Phase 27 artifacts document tested fault scenarios and limitations.
