ALTER TABLE action_predictions ADD COLUMN model_version TEXT NOT NULL DEFAULT '';
CREATE INDEX action_predictions_case_created_idx ON action_predictions(case_id, created_at DESC);
