package store

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/modelregistry"
	"revenue-recovery/backend/internal/recovery"
	"time"
)

const modelColumns = `e.id,e.model_version,e.model_type,e.feature_version,e.training_dataset_version,e.algorithm,e.training_timestamp,e.validation_metrics,e.calibration_metrics,e.artifact_uri,e.artifact_hash,e.created_at,(SELECT status FROM model_registry_status_events WHERE model_registry_id=e.id ORDER BY occurred_at DESC,id DESC LIMIT 1)`

func scanModel(row rowScanner) (modelregistry.Entry, error) {
	var e modelregistry.Entry
	err := row.Scan(&e.ID, &e.ModelVersion, &e.ModelType, &e.FeatureVersion, &e.TrainingDatasetVersion, &e.Algorithm, &e.TrainingTimestamp, &e.ValidationMetrics, &e.CalibrationMetrics, &e.ArtifactURI, &e.ArtifactHash, &e.CreatedAt, &e.Status)
	return e, err
}
func (p *Postgres) GetModelEntry(ctx context.Context, registryID domain.ID) (modelregistry.Entry, error) {
	entry, err := scanModel(p.pool.QueryRow(ctx, `SELECT `+modelColumns+` FROM model_registry_entries e WHERE e.id=$1`, registryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return entry, recovery.ErrNotFound
	}
	return entry, err
}
func (p *Postgres) CreateModelCandidate(ctx context.Context, entry modelregistry.Entry, status modelregistry.StatusInput) (modelregistry.Entry, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return entry, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO model_registry_entries(id,model_version,model_type,feature_version,training_dataset_version,algorithm,training_timestamp,validation_metrics,calibration_metrics,artifact_uri,artifact_hash,created_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, entry.ID, entry.ModelVersion, entry.ModelType, entry.FeatureVersion, entry.TrainingDatasetVersion, entry.Algorithm, entry.TrainingTimestamp, entry.ValidationMetrics, entry.CalibrationMetrics, entry.ArtifactURI, entry.ArtifactHash, entry.CreatedAt)
	if err != nil {
		return entry, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO model_registry_status_events(id,model_registry_id,status,reason,actor,occurred_at)VALUES($1,$2,'CANDIDATE',$3,$4,$5)`, id.New(), entry.ID, status.Reason, status.Actor, entry.CreatedAt)
	if err != nil {
		return entry, err
	}
	entry.Status = "CANDIDATE"
	return entry, tx.Commit(ctx)
}
func (p *Postgres) TransitionModelStatus(ctx context.Context, registryID domain.ID, input modelregistry.StatusInput, now time.Time) (modelregistry.Entry, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return modelregistry.Entry{}, err
	}
	defer tx.Rollback(ctx)
	var current, modelType string
	if err = tx.QueryRow(ctx, `SELECT e.model_type,(SELECT status FROM model_registry_status_events WHERE model_registry_id=e.id ORDER BY occurred_at DESC,id DESC LIMIT 1) FROM model_registry_entries e WHERE e.id=$1 FOR UPDATE`, registryID).Scan(&modelType, &current); err != nil {
		return modelregistry.Entry{}, err
	}
	allowed := map[string]map[string]bool{"CANDIDATE": {"APPROVED": true, "REJECTED": true}, "APPROVED": {"ACTIVE": true, "REJECTED": true}, "ACTIVE": {"RETIRED": true}}
	if !allowed[current][input.Status] {
		return modelregistry.Entry{}, recovery.ErrConflict
	}
	if input.Status == "ACTIVE" {
		rows, queryErr := tx.Query(ctx, `SELECT e.id FROM model_registry_entries e WHERE e.model_type=$1 AND e.id<>$2 AND (SELECT status FROM model_registry_status_events WHERE model_registry_id=e.id ORDER BY occurred_at DESC,id DESC LIMIT 1)='ACTIVE'`, modelType, registryID)
		if queryErr != nil {
			return modelregistry.Entry{}, queryErr
		}
		activeIDs := []domain.ID{}
		for rows.Next() {
			var old domain.ID
			if queryErr = rows.Scan(&old); queryErr != nil {
				rows.Close()
				return modelregistry.Entry{}, queryErr
			}
			activeIDs = append(activeIDs, old)
		}
		rows.Close()
		if queryErr = rows.Err(); queryErr != nil {
			return modelregistry.Entry{}, queryErr
		}
		for _, old := range activeIDs {
			_, queryErr = tx.Exec(ctx, `INSERT INTO model_registry_status_events(id,model_registry_id,status,reason,actor,occurred_at)VALUES($1,$2,'RETIRED',$3,$4,$5)`, id.New(), old, "superseded by "+string(registryID), input.Actor, now)
			if queryErr != nil {
				return modelregistry.Entry{}, queryErr
			}
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO model_registry_status_events(id,model_registry_id,status,reason,actor,occurred_at)VALUES($1,$2,$3,$4,$5,$6)`, id.New(), registryID, input.Status, input.Reason, input.Actor, now)
	if err != nil {
		return modelregistry.Entry{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return modelregistry.Entry{}, err
	}
	return p.GetModelEntry(ctx, registryID)
}
