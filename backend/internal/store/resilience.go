package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"revenue-recovery/backend/internal/recovery"
	"revenue-recovery/backend/internal/resilience"
)

func (p *Postgres) SaveResilienceRun(ctx context.Context, run resilience.Run) error {
	encoded, err := json.Marshal(run.Result)
	if err != nil {
		return err
	}
	_, err = p.pool.Exec(ctx, `INSERT INTO resilience_evaluation_runs(id,suite,environment,scenario,fault_mode,passed,provider_effect_count,execution_attempt_count,result,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, run.ID, run.Result.Suite, run.Environment, run.Result.Scenario, run.Result.FaultMode, run.Result.Passed, run.Result.ProviderEffectCount, run.Result.ExecutionAttemptCount, encoded, run.StartedAt)
	return err
}

func (p *Postgres) GetResilienceRun(ctx context.Context, id string) (resilience.Run, error) {
	var run resilience.Run
	var encoded json.RawMessage
	err := p.pool.QueryRow(ctx, `SELECT id,environment,result,created_at FROM resilience_evaluation_runs WHERE id=$1`, id).Scan(&run.ID, &run.Environment, &encoded, &run.StartedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, recovery.ErrNotFound
	}
	if err != nil {
		return run, err
	}
	if err = json.Unmarshal(encoded, &run.Result); err != nil {
		return run, err
	}
	run.CompletedAt = run.StartedAt
	return run, nil
}
