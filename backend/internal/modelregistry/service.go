package modelregistry

import (
	"context"
	"encoding/json"
	"errors"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"time"
)

type Entry struct {
	ID                     domain.ID       `json:"registry_id"`
	ModelVersion           string          `json:"model_version"`
	ModelType              string          `json:"model_type"`
	FeatureVersion         string          `json:"feature_version"`
	TrainingDatasetVersion string          `json:"training_dataset_version"`
	Algorithm              string          `json:"algorithm"`
	TrainingTimestamp      time.Time       `json:"training_timestamp"`
	ValidationMetrics      json.RawMessage `json:"validation_metrics"`
	CalibrationMetrics     json.RawMessage `json:"calibration_metrics"`
	ArtifactURI            string          `json:"artifact_uri"`
	ArtifactHash           string          `json:"artifact_hash"`
	Status                 string          `json:"status"`
	CreatedAt              time.Time       `json:"created_at"`
}
type StatusInput struct {
	Status string          `json:"status"`
	Reason string          `json:"reason"`
	Actor  json.RawMessage `json:"actor"`
}
type Store interface {
	CreateModelCandidate(context.Context, Entry, StatusInput) (Entry, error)
	GetModelEntry(context.Context, domain.ID) (Entry, error)
	TransitionModelStatus(context.Context, domain.ID, StatusInput, time.Time) (Entry, error)
}
type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }
func (s *Service) Create(ctx context.Context, entry Entry, actor json.RawMessage) (Entry, error) {
	if entry.ModelVersion == "" || entry.ModelType == "" || entry.FeatureVersion == "" || entry.TrainingDatasetVersion == "" || entry.Algorithm == "" || entry.ArtifactURI == "" || entry.ArtifactHash == "" {
		return entry, errors.New("complete version, dataset, algorithm and artifact metadata are required")
	}
	if !json.Valid(entry.ValidationMetrics) || !json.Valid(entry.CalibrationMetrics) {
		return entry, errors.New("validation and calibration metrics must be valid JSON")
	}
	if len(actor) == 0 || !json.Valid(actor) {
		return entry, errors.New("actor is required")
	}
	entry.ID = domain.ID(id.New())
	entry.CreatedAt = s.now().UTC()
	if entry.TrainingTimestamp.IsZero() {
		entry.TrainingTimestamp = entry.CreatedAt
	}
	return s.store.CreateModelCandidate(ctx, entry, StatusInput{Status: "CANDIDATE", Reason: "candidate registered; explicit review required", Actor: actor})
}
func (s *Service) Get(ctx context.Context, id domain.ID) (Entry, error) {
	return s.store.GetModelEntry(ctx, id)
}
func (s *Service) Transition(ctx context.Context, id domain.ID, input StatusInput) (Entry, error) {
	if input.Reason == "" || len(input.Actor) == 0 || !json.Valid(input.Actor) {
		return Entry{}, errors.New("reason and actor are required")
	}
	return s.store.TransitionModelStatus(ctx, id, input, s.now().UTC())
}
