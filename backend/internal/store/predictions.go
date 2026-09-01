package store

import (
	"context"
	"encoding/json"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
)

func (p *Postgres) SaveActionPredictions(ctx context.Context, predictions []domain.ActionPrediction) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if len(predictions) == 0 {
		return tx.Commit(ctx)
	}
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM recovery_cases WHERE id=$1 FOR UPDATE`, predictions[0].CaseID).Scan(&locked); err != nil {
		return err
	}
	sequence, err := nextSequence(ctx, tx, predictions[0].CaseID)
	if err != nil {
		return err
	}
	for _, v := range predictions {
		_, err = tx.Exec(ctx, `INSERT INTO action_predictions(id,case_id,action_id,action_type,recovery_probability,natural_recovery_probability,incremental_uplift,expected_net_value_minor,model_version_id,model_version,feature_version,explanation,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, v.ID, v.CaseID, v.ActionID, v.ActionType, v.RecoveryProbability, v.NaturalRecoveryProbability, v.IncrementalUplift, v.ExpectedNetValueMinor, v.ModelVersionID, v.ModelVersion, v.FeatureVersion, v.Explanation, v.CreatedAt)
		if err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"prediction_id": v.ID, "action": v.ActionType, "recovery_probability": v.RecoveryProbability, "natural_recovery_probability": v.NaturalRecoveryProbability, "incremental_uplift": v.IncrementalUplift, "feature_version": v.FeatureVersion})
		event := domain.RecoveryEvent{ID: domain.ID(id.New()), CaseID: v.CaseID, Sequence: sequence, Type: domain.EventActionPredicted, Timestamp: v.CreatedAt, Actor: domain.Actor{Type: "MODEL", ID: v.ModelVersion}, Payload: payload, ModelVersion: v.ModelVersion, CorrelationID: string(v.ID)}
		if err = insertEvent(ctx, tx, event); err != nil {
			return err
		}
		sequence++
	}
	return tx.Commit(ctx)
}
