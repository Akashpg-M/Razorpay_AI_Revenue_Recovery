from __future__ import annotations

import argparse
import json
from pathlib import Path

import joblib

from evaluation.environment import file_sha256, load_jsonl
from prediction.training import examples, metrics


def evaluate(artifact: Path, dataset: Path, output: Path) -> dict:
    rows = load_jsonl(dataset, "test")
    bundle = joblib.load(artifact)
    x, y, metadata = examples(rows)
    result = {
        "model_version": bundle["model_version"], "feature_version": bundle["feature_version"],
        "algorithm": bundle["algorithm"], "dataset_sha256": file_sha256(dataset),
        "dataset_size": len(rows), "split": "test", "metrics": metrics(bundle["model"], x, y, metadata),
    }
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return result


def main() -> None:
    parser = argparse.ArgumentParser(description="Final held-out evaluation of a frozen outcome model")
    parser.add_argument("--artifact", type=Path, required=True)
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--output", type=Path, default=Path("evaluation/results/outcome_v1_test.json"))
    args = parser.parse_args()
    print(json.dumps(evaluate(args.artifact, args.dataset, args.output), indent=2, sort_keys=True))


if __name__ == "__main__": main()
