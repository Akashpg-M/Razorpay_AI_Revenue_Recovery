import unittest

from evaluation.ablations import CONFIGURATIONS, _allowed, _neutralize


OBSERVABLE = {"leak_type":"FAILED_SUBSCRIPTION","merchant_type":"SAAS","customer_segment":"HIGH_VALUE",
              "subscription_tenure_days":300,"failure_count_90d":2,"past_recovery_actions":[],
              "contact_history":{"last_7d":1},"payment_history":{"failed_count":2,"successful_count":8,"success_rate":.8}}


class AblationTests(unittest.TestCase):
    def test_all_required_ablations_are_declared(self):
        self.assertEqual(len(CONFIGURATIONS), 10)

    def test_customer_ablation_does_not_remove_merchant_context(self):
        value = _neutralize(OBSERVABLE, "without_customer_context")
        self.assertEqual(value["merchant_type"], "SAAS")
        self.assertEqual(value["subscription_tenure_days"], 0)

    def test_policy_ablation_is_the_only_one_that_bypasses_fatigue_limit(self):
        exhausted = {**OBSERVABLE, "contact_history":{"last_7d":5}}
        self.assertNotIn("SEND_REMINDER", _allowed(exhausted, "full_nba_agent_v1"))
        self.assertIn("SEND_REMINDER", _allowed(exhausted, "without_policy_aware_optimization"))
