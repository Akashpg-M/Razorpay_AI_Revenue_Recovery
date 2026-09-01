package resilience

import "testing"

func TestAuthorizationSuiteBlocksEveryRestrictedExternalEffect(t *testing.T) {
	for _, result := range RunAuthorizationSuite() {
		if !result.Passed {
			t.Fatalf("scenario %s failed: %+v", result.Scenario, result)
		}
		if result.Scenario != "valid_control" && result.ExternalCallCount != 0 {
			t.Fatalf("unsafe external call in %s", result.Scenario)
		}
	}
}

func TestAmbiguousProviderSuccessIsReconciledWithoutDuplicate(t *testing.T) {
	result := RunFaultScenario("network_timeout_after_provider_succeeds")
	if !result.Passed || result.ProviderEffectCount != 1 || result.ExternalCallCount != 1 {
		t.Fatalf("ambiguous outcome was not reconciled: %+v", result)
	}
}

func TestRequiredFaultMatrixPasses(t *testing.T) {
	for _, result := range RunFaultSuite() {
		if !result.Passed {
			t.Fatalf("fault scenario %s failed: %+v", result.Scenario, result)
		}
		if result.ProviderEffectCount > 1 {
			t.Fatalf("duplicate provider effect in %s", result.Scenario)
		}
	}
}
