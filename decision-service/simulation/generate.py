from __future__ import annotations

import argparse
import json
from pathlib import Path

from simulation.generator import SimulationConfig, generate_dataset


def distribution(value: str) -> dict[str, float]:
    candidate = Path(value)
    try:
        raw = candidate.read_text(encoding="utf-8") if candidate.is_file() else value
    except OSError:
        raw = value
    parsed = json.loads(raw)
    if not isinstance(parsed, dict):
        raise argparse.ArgumentTypeError("distribution must be a JSON object or a path to one")
    try:
        return {str(key): float(weight) for key, weight in parsed.items()}
    except (TypeError, ValueError) as error:
        raise argparse.ArgumentTypeError("distribution weights must be numeric") from error


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate a deterministic revenue-recovery dataset")
    parser.add_argument("--seed", type=int, default=42)
    parser.add_argument("--dataset-size", type=int, default=5000)
    parser.add_argument("--subscription-share", type=float, default=0.70)
    parser.add_argument("--merchant-mix", type=distribution)
    parser.add_argument("--subscription-failure-distribution", type=distribution)
    parser.add_argument("--checkout-failure-distribution", type=distribution)
    parser.add_argument("--output-dir", type=Path, default=Path("simulation/data"))
    args = parser.parse_args()
    defaults = SimulationConfig()
    config = SimulationConfig(
        seed=args.seed,
        dataset_size=args.dataset_size,
        subscription_share=args.subscription_share,
        merchant_mix=args.merchant_mix or defaults.merchant_mix,
        subscription_failure_distribution=(
            args.subscription_failure_distribution or defaults.subscription_failure_distribution
        ),
        checkout_failure_distribution=(
            args.checkout_failure_distribution or defaults.checkout_failure_distribution
        ),
    )
    report = generate_dataset(config, args.output_dir)
    print(json.dumps(report, indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
