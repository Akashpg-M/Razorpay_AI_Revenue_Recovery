from __future__ import annotations

import argparse
import json
from pathlib import Path
from typing import Any

from evaluation.environment import file_sha256, load_jsonl
from evaluation.metrics import evaluate
from evaluation.strategies import ContextualRetry, FixedRetry, NoRecovery, RuleBased


def run(dataset: Path, train_dataset: Path, seed: int, output_dir: Path) -> dict[str, Any]:
    test_rows = load_jsonl(dataset, required_split="test")
    train_rows = load_jsonl(train_dataset, required_split="train")
    contextual = ContextualRetry().fit(train_rows, seed)
    strategies = (NoRecovery(), FixedRetry(), RuleBased(), contextual)
    dataset_hash = file_sha256(dataset)
    output_dir.mkdir(parents=True, exist_ok=True)
    results = []
    for strategy in strategies:
        result = evaluate(test_rows, strategy, seed)
        result.update({
            "dataset_path": str(dataset),
            "dataset_sha256": dataset_hash,
            "dataset_size": len(test_rows),
            "simulation_version": test_rows[0]["simulation_version"],
            "evaluation_timestamp": "2026-01-01T00:00:00+00:00",
        })
        target = output_dir / f"baseline_{strategy.name}.json"
        target.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        results.append(result)
    comparison = {"dataset_sha256": dataset_hash, "seed": seed, "baselines": results}
    (output_dir / "baseline_comparison.json").write_text(json.dumps(comparison, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    (output_dir / "baseline_comparison.md").write_text(_markdown(results), encoding="utf-8")
    return comparison


def _markdown(results: list[dict[str, Any]]) -> str:
    lines = [
        "# Baseline comparison", "",
        "| Baseline | Recovered (minor) | Rate | Attempts | Contacts | Avg latency (h) | Cost (minor) |",
        "|---|---:|---:|---:|---:|---:|---:|",
    ]
    for result in results:
        metric = result["metrics"]["aggregate"]
        lines.append(
            f"| {result['baseline']} | {metric['total_revenue_recovered_minor']} | {metric['recovery_rate']:.2%} | "
            f"{metric['recovery_attempts']} | {metric['customer_contacts']} | "
            f"{metric['average_recovery_latency_hours']:.2f} | {metric['intervention_cost_minor']} |"
        )
    return "\n".join(lines) + "\n"


def main() -> None:
    parser = argparse.ArgumentParser(description="Run all recovery baselines on one held-out dataset")
    parser.add_argument("--dataset", type=Path, required=True)
    parser.add_argument("--train-dataset", type=Path, required=True)
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--output-dir", type=Path, default=Path("evaluation/results"))
    args = parser.parse_args()
    comparison = run(args.dataset, args.train_dataset, args.seed, args.output_dir)
    print(json.dumps(comparison, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
