# Policy engine

Eligibility removes impossible actions before scoring. The economic gate rejects non-positive value. Final policy rechecks terminal/recovered state, deadline, stale version, allowed action/channel, opt-out, retry/contact limits, cooldowns, promises, mandate/payment validity, discounts, quiet hours, and channel availability. High value and low confidence escalate.

An operator approval triggers a fresh context and policy evaluation inside a transaction. A changed case version, merchant policy, deadline, or terminal outcome produces `STALE_APPROVAL` and no scheduled action. Policy decisions and checks are replayable.
