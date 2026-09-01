package budget

import (
	"context"
	"encoding/json"
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/id"
	"revenue-recovery/backend/internal/portfolio"
	"sort"
	"time"
)

const GreedyVersion = "budget-greedy-v1"
const FCFSVersion = "budget-fcfs-v1"

type Limits struct {
	SpendMinor    int64 `json:"spend_minor"`
	Contacts      int   `json:"contacts"`
	Retries       int   `json:"retries"`
	DiscountMinor int64 `json:"discount_minor"`
	HumanReviews  int   `json:"human_reviews"`
}
type Allocation struct {
	ID                            domain.ID         `json:"allocation_id"`
	RunID                         string            `json:"run_id"`
	CaseID                        domain.ID         `json:"case_id"`
	DecisionID                    domain.ID         `json:"decision_id"`
	Action                        domain.ActionType `json:"action"`
	ExpectedIncrementalValueMinor int64             `json:"expected_incremental_value_minor"`
	ExpectedNERVMinor             int64             `json:"expected_nerv_minor"`
	ExpectedCostMinor             int64             `json:"expected_cost_minor"`
	ResourceConsumption           json.RawMessage   `json:"resource_consumption"`
	Rank                          int               `json:"allocation_rank"`
	Included                      bool              `json:"included"`
	ExclusionReason               string            `json:"exclusion_reason"`
	CreatedAt                     time.Time         `json:"created_at"`
}
type Run struct {
	ID               string       `json:"run_id"`
	MerchantID       domain.ID    `json:"merchant_id"`
	AlgorithmVersion string       `json:"algorithm_version"`
	PriorityRunID    string       `json:"priority_run_id"`
	Budget           Limits       `json:"budget"`
	Totals           Limits       `json:"totals"`
	CreatedAt        time.Time    `json:"created_at"`
	Allocations      []Allocation `json:"allocations"`
}

func resources(action domain.ActionType) (cost, discount int64, contacts, retries, reviews int) {
	switch action {
	case domain.ActionRetryNow, domain.ActionRetryLater:
		cost = 35
		retries = 1
	case domain.ActionSendReminder:
		cost = 25
		contacts = 1
	case domain.ActionSendPaymentLink, domain.ActionSendCheckoutRecoveryLink:
		cost = 70
		contacts = 1
	case domain.ActionRetention:
		cost = 100
		discount = 3000
		contacts = 1
	case domain.ActionEscalateToHuman:
		reviews = 1
	}
	return
}
func Allocate(items []portfolio.Item, limits Limits, algorithm string, now time.Time, merchantID domain.ID, priorityRunID string) Run {
	ordered := append([]portfolio.Item(nil), items...)
	if algorithm == GreedyVersion {
		sort.SliceStable(ordered, func(i, j int) bool {
			ci, _, _, _, _ := resources(ordered[i].Action)
			cj, _, _, _, _ := resources(ordered[j].Action)
			di := ci
			if di < 1 {
				di = 1
			}
			dj := cj
			if dj < 1 {
				dj = 1
			}
			left := ordered[i].ExpectedNERVMinor * dj
			right := ordered[j].ExpectedNERVMinor * di
			if left != right {
				return left > right
			}
			return ordered[i].Rank < ordered[j].Rank
		})
	} else {
		algorithm = FCFSVersion
		sort.SliceStable(ordered, func(i, j int) bool {
			if !ordered[i].ArrivalAt.Equal(ordered[j].ArrivalAt) {
				return ordered[i].ArrivalAt.Before(ordered[j].ArrivalAt)
			}
			return ordered[i].CaseID < ordered[j].CaseID
		})
	}
	run := Run{ID: id.New(), MerchantID: merchantID, AlgorithmVersion: algorithm, PriorityRunID: priorityRunID, Budget: limits, CreatedAt: now.UTC()}
	for index, item := range ordered {
		cost, discount, contacts, retries, reviews := resources(item.Action)
		included := true
		reason := ""
		switch {
		case run.Totals.SpendMinor+cost > limits.SpendMinor:
			included = false
			reason = "SPEND_BUDGET_EXHAUSTED"
		case run.Totals.Contacts+contacts > limits.Contacts:
			included = false
			reason = "CONTACT_CAPACITY_EXHAUSTED"
		case run.Totals.Retries+retries > limits.Retries:
			included = false
			reason = "RETRY_CAPACITY_EXHAUSTED"
		case run.Totals.DiscountMinor+discount > limits.DiscountMinor:
			included = false
			reason = "DISCOUNT_BUDGET_EXHAUSTED"
		case run.Totals.HumanReviews+reviews > limits.HumanReviews:
			included = false
			reason = "HUMAN_REVIEW_CAPACITY_EXHAUSTED"
		}
		if included {
			run.Totals.SpendMinor += cost
			run.Totals.Contacts += contacts
			run.Totals.Retries += retries
			run.Totals.DiscountMinor += discount
			run.Totals.HumanReviews += reviews
		}
		resource, _ := json.Marshal(Limits{SpendMinor: cost, Contacts: contacts, Retries: retries, DiscountMinor: discount, HumanReviews: reviews})
		run.Allocations = append(run.Allocations, Allocation{ID: domain.ID(id.New()), RunID: run.ID, CaseID: item.CaseID, DecisionID: item.DecisionID, Action: item.Action, ExpectedIncrementalValueMinor: item.ExpectedIncrementalValueMinor, ExpectedNERVMinor: item.ExpectedNERVMinor, ExpectedCostMinor: cost, ResourceConsumption: resource, Rank: index + 1, Included: included, ExclusionReason: reason, CreatedAt: now.UTC()})
	}
	return run
}

type Store interface {
	LoadPriorityRun(context.Context, string) ([]portfolio.Item, error)
	SaveBudgetRun(context.Context, Run) error
}
type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }
func (s *Service) Run(ctx context.Context, merchantID domain.ID, priorityRunID, algorithm string, limits Limits) (Run, error) {
	items, err := s.store.LoadPriorityRun(ctx, priorityRunID)
	if err != nil {
		return Run{}, err
	}
	run := Allocate(items, limits, algorithm, s.now().UTC(), merchantID, priorityRunID)
	if err = s.store.SaveBudgetRun(ctx, run); err != nil {
		return Run{}, err
	}
	return run, nil
}
