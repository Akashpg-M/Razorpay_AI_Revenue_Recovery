package economicgate

import (
	"revenue-recovery/backend/internal/domain"
	"revenue-recovery/backend/internal/optimizer"
	"testing"
	"time"
)

func TestThresholdBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name             string
		value, threshold int64
		decision, reason string
	}{{"above", 101, 100, "ALLOW", "ABOVE_MINIMUM_VALUE"}, {"equal", 100, 100, "ALLOW", "AT_MINIMUM_VALUE"}, {"below", 99, 100, "BLOCK", "BELOW_MINIMUM_VALUE"}, {"negative", -1, 0, "BLOCK", "ECONOMICALLY_UNJUSTIFIED"}} {
		t.Run(tc.name, func(t *testing.T) {
			r := Evaluate("d", "c", optimizer.Candidate{Action: domain.ActionSendReminder, NERVMinor: tc.value}, tc.threshold, time.Now())
			if r.Decision != tc.decision || r.Reason != tc.reason {
				t.Fatalf("%+v", r)
			}
		})
	}
}
func TestMissingThresholdDefaultsToZero(t *testing.T) {
	r := Evaluate("d", "c", optimizer.Candidate{Action: domain.ActionRetryLater, NERVMinor: 1}, 0, time.Now())
	if r.Decision != "ALLOW" {
		t.Fatalf("%+v", r)
	}
}
