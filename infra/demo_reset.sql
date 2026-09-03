BEGIN;
-- The normal application role can never mutate append-only evidence. This
-- script is invoked only by the explicitly guarded local demo reset and uses
-- the local PostgreSQL owner for this transaction-scoped maintenance window.
SET LOCAL session_replication_role = replica;
CREATE TEMP TABLE reset_cases AS
SELECT id FROM recovery_cases
WHERE source_reference LIKE 'checkout_%' OR source_reference LIKE 'pay_demo_%';

DELETE FROM email_deliveries WHERE scheduled_action_id IN (SELECT id FROM scheduled_actions WHERE case_id IN (SELECT id FROM reset_cases));
DELETE FROM feedback_records WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM human_review_records WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM provider_action_references WHERE action_id IN (SELECT id FROM recovery_actions WHERE case_id IN (SELECT id FROM reset_cases));
DELETE FROM budget_allocations WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM portfolio_priority_snapshots WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM action_predictions WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM recovery_attributions WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM executions WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM promise_checks WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM promise_events WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM promises_to_pay WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM scheduled_actions WHERE case_id IN (SELECT id FROM reset_cases);
SET CONSTRAINTS policy_decisions_action_fk DEFERRED;
DELETE FROM policy_decisions WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM recovery_actions WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM policy_evaluations WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM economic_gate_evaluations WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM recovery_decision_candidates WHERE decision_id IN (SELECT id FROM recovery_decisions WHERE case_id IN (SELECT id FROM reset_cases));
DELETE FROM natural_recovery_predictions WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM recovery_decisions WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM recovery_events WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM customer_responses WHERE case_id IN (SELECT id FROM reset_cases);
DELETE FROM recovery_cases WHERE id IN (SELECT id FROM reset_cases);
SET LOCAL session_replication_role = origin;
COMMIT;
