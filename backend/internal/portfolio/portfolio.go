package portfolio

import (
	"context"
	"encoding/json"
	"math/big"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"sort"
	"time"
)

const AlgorithmVersion = "portfolio-priority-v1"

type Candidate struct {
	MerchantID                    domain.ID
	CaseID                        domain.ID
	DecisionID                    domain.ID
	Action                        domain.ActionType
	AmountAtRiskMinor             int64
	ExpectedIncrementalValueMinor int64
	ExpectedNERVMinor             int64
	Deadline                      time.Time
	ArrivalAt                     time.Time
	RecoverabilityBPS             int64
}
type Item struct {
	ID                            domain.ID         `json:"priority_id"`
	RunID                         string            `json:"run_id"`
	MerchantID                    domain.ID         `json:"merchant_id"`
	CaseID                        domain.ID         `json:"case_id"`
	DecisionID                    domain.ID         `json:"decision_id"`
	Action                        domain.ActionType `json:"action"`
	AmountAtRiskMinor             int64             `json:"amount_at_risk_minor"`
	ExpectedIncrementalValueMinor int64             `json:"expected_incremental_value_minor"`
	ExpectedNERVMinor             int64             `json:"expected_nerv_minor"`
	UrgencyBPS                    int64             `json:"urgency_bps"`
	RecoverabilityBPS             int64             `json:"recoverability_bps"`
	PriorityScoreMinor            int64             `json:"priority_score_minor"`
	Rank                          int               `json:"rank"`
	Explanation                   json.RawMessage   `json:"explanation"`
	AlgorithmVersion              string            `json:"algorithm_version"`
	CreatedAt                     time.Time         `json:"created_at"`
	ArrivalAt                     time.Time         `json:"case_arrival_at"`
}

func Rank(candidates []Candidate, now time.Time, runID string) []Item {
	items := make([]Item, 0, len(candidates))
	for _, c := range candidates {
		hours := c.Deadline.Sub(now).Hours()
		urgency := int64(7500)
		if hours <= 1 {
			urgency = 15000
		} else if hours <= 6 {
			urgency = 12500
		} else if hours <= 24 {
			urgency = 10000
		}
		recoverability := c.RecoverabilityBPS
		if recoverability < 0 {
			recoverability = 0
		}
		if recoverability > 10000 {
			recoverability = 10000
		}
		score := mulDiv(mulDiv(c.ExpectedNERVMinor, urgency, 10000), recoverability, 10000)
		explanation, _ := json.Marshal(map[string]any{"formula": "expected_nerv * urgency * recoverability", "hours_until_deadline": hours, "case_created_at": c.ArrivalAt})
		items = append(items, Item{ID: domain.ID(id.New()), RunID: runID, MerchantID: c.MerchantID, CaseID: c.CaseID, DecisionID: c.DecisionID, Action: c.Action, AmountAtRiskMinor: c.AmountAtRiskMinor, ExpectedIncrementalValueMinor: c.ExpectedIncrementalValueMinor, ExpectedNERVMinor: c.ExpectedNERVMinor, UrgencyBPS: urgency, RecoverabilityBPS: recoverability, PriorityScoreMinor: score, Explanation: explanation, AlgorithmVersion: AlgorithmVersion, CreatedAt: now.UTC(), ArrivalAt: c.ArrivalAt})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].PriorityScoreMinor != items[j].PriorityScoreMinor {
			return items[i].PriorityScoreMinor > items[j].PriorityScoreMinor
		}
		if items[i].ExpectedNERVMinor != items[j].ExpectedNERVMinor {
			return items[i].ExpectedNERVMinor > items[j].ExpectedNERVMinor
		}
		return items[i].CaseID < items[j].CaseID
	})
	for i := range items {
		items[i].Rank = i + 1
	}
	return items
}
func mulDiv(a, b, d int64) int64 {
	v := new(big.Int).Mul(big.NewInt(a), big.NewInt(b))
	v.Quo(v, big.NewInt(d))
	return v.Int64()
}

type Store interface {
	LoadPortfolioCandidates(context.Context, domain.ID) ([]Candidate, error)
	SavePortfolioPriority(context.Context, []Item) error
}
type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }
func (s *Service) Run(ctx context.Context, merchantID domain.ID) (string, []Item, error) {
	candidates, err := s.store.LoadPortfolioCandidates(ctx, merchantID)
	if err != nil {
		return "", nil, err
	}
	runID := id.New()
	items := Rank(candidates, s.now().UTC(), runID)
	if err = s.store.SavePortfolioPriority(ctx, items); err != nil {
		return "", nil, err
	}
	return runID, items, nil
}
