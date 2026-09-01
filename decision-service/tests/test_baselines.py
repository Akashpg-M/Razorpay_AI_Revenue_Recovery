import json
import tempfile
import unittest
from pathlib import Path

from evaluation.baselines import run
from evaluation.environment import load_jsonl
from evaluation.strategies import ContextualRetry, RuleBased


DATA = Path(__file__).parents[1] / "simulation" / "data"


class BaselineTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.train = load_jsonl(DATA / "train.jsonl", "train")
        cls.test = load_jsonl(DATA / "test.jsonl", "test")

    def test_every_baseline_uses_identical_test_population_and_is_deterministic(self):
        with tempfile.TemporaryDirectory() as first, tempfile.TemporaryDirectory() as second:
            a = run(DATA / "test.jsonl", DATA / "train.jsonl", 42, Path(first))
            b = run(DATA / "test.jsonl", DATA / "train.jsonl", 42, Path(second))
            self.assertEqual(a, b)
            hashes = {result["dataset_sha256"] for result in a["baselines"]}
            sizes = {result["dataset_size"] for result in a["baselines"]}
            self.assertEqual(len(hashes), 1)
            self.assertEqual(sizes, {750})

    def test_contextual_retry_rejects_test_data_for_fit(self):
        with self.assertRaises(ValueError):
            ContextualRetry().fit(self.test, 42)

    def test_contextual_retry_never_selects_non_retry_action(self):
        strategy = ContextualRetry().fit(self.train, 42)
        selected = {strategy.decide(row["observable"]).action for row in self.test}
        self.assertTrue(selected <= strategy.ALLOWED)

    def test_rules_are_explicit_and_versioned(self):
        strategy = RuleBased()
        self.assertEqual(strategy.version, "rules-v1")
        selected = {strategy.decide(row["observable"]).action for row in self.test}
        self.assertNotIn("", selected)

    def test_observable_contract_does_not_contain_hidden_features(self):
        hidden_names = {"liquidity_pattern", "retry_responsiveness", "natural_recovery_probability", "churn_intent"}
        for row in self.test[:50]:
            self.assertTrue(hidden_names.isdisjoint(row["observable"]))


if __name__ == "__main__":
    unittest.main()
