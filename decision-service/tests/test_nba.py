import unittest
from evaluation.nba import rank_candidates

class NBATests(unittest.TestCase):
    def observable(self):
        return {"amount_at_risk_minor": 100_000, "failure_type": "INSUFFICIENT_FUNDS", "contact_history": {"last_7d": 0}}
    def test_wait_beats_negative_incremental_action(self):
        ranked = rank_candidates(self.observable(), {"SEND_REMINDER": .10}, .40)
        self.assertEqual(ranked[0]["action"], "WAIT")
        self.assertLess(ranked[1]["incremental_uplift"], 0)
    def test_ranking_is_deterministic(self):
        predictions = {"SEND_REMINDER": .5, "RETRY_LATER": .5}
        self.assertEqual(rank_candidates(self.observable(), predictions, .2), rank_candidates(self.observable(), predictions, .2))
    def test_integer_minor_unit_arithmetic(self):
        ranked = rank_candidates({**self.observable(), "amount_at_risk_minor": 9_007_199_254_740_993}, {"RETRY_LATER": .3}, .2)
        self.assertIsInstance(ranked[0]["nerv_minor"], int)

if __name__ == "__main__": unittest.main()
