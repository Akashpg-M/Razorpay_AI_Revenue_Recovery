# Operations

Open `/operations` to review escalated and exhausted cases ordered by deadline, expected NERV, and case ID. Filters cover category, merchant, status, priority, leak type, value, and deadline. Approve, reject, defer, and stop require operator identity, reason, and a unique idempotency key.

Approvals are reauthorized against live state and policy. Rejections and deferrals retain the case for review; stop is terminal. Every outcome is immutable and appears in `/recovery/{case_id}` replay. Never put customer contact data in operator notes.
