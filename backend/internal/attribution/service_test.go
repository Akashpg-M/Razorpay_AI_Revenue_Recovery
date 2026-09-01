package attribution

import "testing"

func TestAttributionV2PrecedenceIsExplicitAndExactFirst(t *testing.T) {
	if RuleVersion != "attribution-v2" {
		t.Fatalf("unexpected rule version %q", RuleVersion)
	}
	want := []string{"EXACT_PROVIDER_REFERENCE", "PTP", "RETRY", "DIRECT_ACTION", "NATURAL", "UNKNOWN"}
	if len(Precedence) != len(want) {
		t.Fatalf("precedence length = %d, want %d", len(Precedence), len(want))
	}
	for index := range want {
		if Precedence[index] != want[index] {
			t.Fatalf("precedence[%d] = %q, want %q", index, Precedence[index], want[index])
		}
	}
}

func TestAttributionOverlapResolution(t *testing.T) {
	tests := []struct {
		name       string
		candidates []EvidenceCandidate
		want       Category
	}{
		{"exact provider beats promise", []EvidenceCandidate{{"PTP", Promise}, {"EXACT_PROVIDER_REFERENCE", DirectAction}}, DirectAction},
		{"exact retry beats promise", []EvidenceCandidate{{"PTP", Promise}, {"EXACT_PROVIDER_REFERENCE", Retry}}, Retry},
		{"promise beats temporal retry", []EvidenceCandidate{{"RETRY", Retry}, {"PTP", Promise}}, Promise},
		{"retry beats broad direct window", []EvidenceCandidate{{"DIRECT_ACTION", DirectAction}, {"RETRY", Retry}}, Retry},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ResolveCandidates(test.candidates...); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
		})
	}
}
