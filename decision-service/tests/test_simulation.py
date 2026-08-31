import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from simulation.generator import SimulationConfig, generate_dataset


class SimulationTests(unittest.TestCase):
    def test_same_seed_produces_byte_identical_dataset(self):
        config = SimulationConfig(seed=1234, dataset_size=120)
        with tempfile.TemporaryDirectory() as first, tempfile.TemporaryDirectory() as second:
            generate_dataset(config, first)
            generate_dataset(config, second)
            for name in ("train.jsonl", "validation.jsonl", "test.jsonl", "dataset_report.json"):
                self.assertEqual(self._digest(Path(first) / name), self._digest(Path(second) / name))

    def test_split_and_hidden_feature_contract(self):
        with tempfile.TemporaryDirectory() as directory:
            report = generate_dataset(SimulationConfig(seed=7, dataset_size=101), directory)
            self.assertEqual(report["split_sizes"], {"train": 70, "validation": 15, "test": 16})
            row = json.loads((Path(directory) / "test.jsonl").read_text(encoding="utf-8").splitlines()[0])
            self.assertNotIn("liquidity_pattern", row["observable"])
            self.assertIn("liquidity_pattern", row["_ground_truth"]["hidden_customer"])
            self.assertIn("potential_outcomes", row["_ground_truth"])

    def test_configuration_validation(self):
        with self.assertRaises(ValueError):
            SimulationConfig(dataset_size=0).validate()
        with self.assertRaises(ValueError):
            SimulationConfig(merchant_mix={"A": 0.8}).validate()

    @staticmethod
    def _digest(path: Path) -> str:
        return hashlib.sha256(path.read_bytes()).hexdigest()


if __name__ == "__main__":
    unittest.main()
