package economicgate

import (
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/optimizer"
	"time"
)

const Version = "economic-gate-v1"

type Result struct {
	ID             domain.ID         `json:"gate_id"`
	DecisionID     domain.ID         `json:"decision_id"`
	CaseID         domain.ID         `json:"case_id"`
	Action         domain.ActionType `json:"action"`
	NERVMinor      int64             `json:"net_expected_recovery_value_minor"`
	ThresholdMinor int64             `json:"merchant_minimum_value_threshold_minor"`
	Decision       string            `json:"decision"`
	Reason         string            `json:"reason"`
	GateVersion    string            `json:"gate_version"`
	CreatedAt      time.Time         `json:"created_at"`
}

func Evaluate(decisionID, caseID domain.ID, candidate optimizer.Candidate, threshold int64, now time.Time) Result {
	result := "ALLOW"
	reason := "ABOVE_MINIMUM_VALUE"
	if candidate.Action == domain.ActionWait {
		reason = "WAIT_SELECTED"
	} else if candidate.NERVMinor < 0 {
		result = "BLOCK"
		reason = "ECONOMICALLY_UNJUSTIFIED"
	} else if candidate.NERVMinor < threshold {
		result = "BLOCK"
		reason = "BELOW_MINIMUM_VALUE"
	} else if candidate.NERVMinor == threshold {
		reason = "AT_MINIMUM_VALUE"
	}
	return Result{DecisionID: decisionID, CaseID: caseID, Action: candidate.Action, NERVMinor: candidate.NERVMinor, ThresholdMinor: threshold, Decision: result, Reason: reason, GateVersion: Version, CreatedAt: now.UTC()}
}
