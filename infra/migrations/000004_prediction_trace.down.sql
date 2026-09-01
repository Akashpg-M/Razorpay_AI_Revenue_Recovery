DROP INDEX IF EXISTS action_predictions_case_created_idx;
ALTER TABLE action_predictions DROP COLUMN IF EXISTS model_version;
