from __future__ import annotations

import hashlib
import json
import math
import random
from collections import Counter
from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


SIMULATION_VERSION = "1.0.0"
FEATURE_VERSION = "synthetic-observable-v1"

SUBSCRIPTION_FAILURES = {
    "TEMPORARY_BANK_FAILURE": 0.22,
    "INSUFFICIENT_FUNDS": 0.28,
    "PAYMENT_METHOD_INVALID": 0.16,
    "MANDATE_FAILURE": 0.14,
    "HARD_DECLINE": 0.12,
    "AMBIGUOUS_CUSTOMER_INTENT": 0.08,
}
CHECKOUT_FAILURES = {
    "PAYMENT_FAILURE": 0.24,
    "PAYMENT_FRICTION": 0.21,
    "METHOD_MISMATCH": 0.17,
    "TEMPORARY_ABANDONMENT": 0.19,
    "PRICE_HESITATION": 0.11,
    "LOW_INTENT_ABANDONMENT": 0.08,
}
MERCHANT_MIX = {"SAAS": 0.30, "ECOMMERCE": 0.35, "EDTECH": 0.18, "MEDIA": 0.17}
PAYMENT_METHODS = ("UPI", "CARD", "NETBANKING", "WALLET")

SUBSCRIPTION_ACTIONS = (
    "WAIT", "RETRY_NOW", "RETRY_LATER", "SEND_REMINDER",
    "SEND_PAYMENT_LINK", "REQUEST_PAYMENT_METHOD_UPDATE",
    "SUGGEST_ALTERNATE_METHOD", "WAIT_FOR_PROMISE_TO_PAY", "RETENTION_ACTION",
)
CHECKOUT_ACTIONS = (
    "WAIT", "SEND_REMINDER", "SEND_CHECKOUT_RECOVERY_LINK",
    "SEND_PAYMENT_LINK", "SUGGEST_ALTERNATE_METHOD", "RETENTION_ACTION",
)

ACTION_COST_MINOR = {
    "WAIT": 0, "RETRY_NOW": 35, "RETRY_LATER": 35, "SEND_REMINDER": 25,
    "SEND_PAYMENT_LINK": 45, "SEND_CHECKOUT_RECOVERY_LINK": 45,
    "REQUEST_PAYMENT_METHOD_UPDATE": 60, "SUGGEST_ALTERNATE_METHOD": 55,
    "WAIT_FOR_PROMISE_TO_PAY": 10, "RETENTION_ACTION": 3000,
}


@dataclass(frozen=True)
class SimulationConfig:
    seed: int = 42
    dataset_size: int = 5000
    subscription_share: float = 0.70
    merchant_mix: dict[str, float] = field(default_factory=lambda: dict(MERCHANT_MIX))
    subscription_failure_distribution: dict[str, float] = field(
        default_factory=lambda: dict(SUBSCRIPTION_FAILURES)
    )
    checkout_failure_distribution: dict[str, float] = field(
        default_factory=lambda: dict(CHECKOUT_FAILURES)
    )

    def validate(self) -> None:
        if self.dataset_size < 1:
            raise ValueError("dataset_size must be positive")
        if not 0 <= self.subscription_share <= 1:
            raise ValueError("subscription_share must be between 0 and 1")
        for name, distribution in (
            ("merchant_mix", self.merchant_mix),
            ("subscription_failure_distribution", self.subscription_failure_distribution),
            ("checkout_failure_distribution", self.checkout_failure_distribution),
        ):
            if not distribution or any(value < 0 for value in distribution.values()):
                raise ValueError(f"{name} must contain non-negative weights")
            if not math.isclose(sum(distribution.values()), 1.0, abs_tol=1e-9):
                raise ValueError(f"{name} weights must sum to 1")


def _weighted_choice(rng: random.Random, values: dict[str, float]) -> str:
    point = rng.random()
    cumulative = 0.0
    for value, weight in values.items():
        cumulative += weight
        if point <= cumulative:
            return value
    return next(reversed(values))


def _beta(rng: random.Random, alpha: float, beta: float) -> float:
    return round(rng.betavariate(alpha, beta), 6)


def _bounded(value: float, lower: float = 0.005, upper: float = 0.95) -> float:
    return round(max(lower, min(upper, value)), 6)


def _stable_uniform(seed: int, case_id: str, action: str, suffix: str) -> float:
    digest = hashlib.sha256(f"{seed}:{case_id}:{action}:{suffix}".encode()).digest()
    return int.from_bytes(digest[:8], "big") / 2**64


def _case_id(seed: int, index: int) -> str:
    digest = hashlib.sha256(f"recovery-simulation:{seed}:{index}".encode()).hexdigest()
    return f"sim-{digest[:8]}-{digest[8:12]}-{digest[12:16]}-{digest[16:28]}"


def _hidden_customer(rng: random.Random) -> dict[str, Any]:
    liquidity = _weighted_choice(rng, {"STABLE": 0.48, "PAYDAY_CYCLICAL": 0.32, "VOLATILE": 0.20})
    return {
        "liquidity_pattern": liquidity,
        "retry_responsiveness": _beta(rng, 2.2, 2.3),
        "payment_link_responsiveness": _beta(rng, 2.0, 2.5),
        "reminder_responsiveness": _beta(rng, 1.8, 3.0),
        "contact_sensitivity": _beta(rng, 1.6, 3.2),
        "promise_reliability": _beta(rng, 2.8, 1.9),
        "natural_recovery_probability": _beta(rng, 1.5, 4.4),
        "payment_method_preference": rng.choice(PAYMENT_METHODS),
        "churn_intent": _beta(rng, 1.25, 4.8),
    }


def _observable_customer(rng: random.Random, hidden: dict[str, Any], merchant_type: str) -> dict[str, Any]:
    successful = rng.randint(0, 42)
    failed = rng.randint(0, min(9, 2 + successful // 5))
    tenure = rng.randint(7, 1460)
    payment_method = hidden["payment_method_preference"] if rng.random() < 0.72 else rng.choice(PAYMENT_METHODS)
    contacts = rng.randint(0, 5)
    return {
        "payment_history": {
            "successful_count": successful,
            "failed_count": failed,
            "success_rate": round(successful / max(1, successful + failed), 4),
        },
        "failure_count_90d": failed,
        "subscription_tenure_days": tenure,
        "payment_method": payment_method,
        "past_recovery_actions": rng.choices(
            ["NONE", "RETRY_LATER", "SEND_REMINDER", "SEND_PAYMENT_LINK"],
            weights=[0.45, 0.25, 0.17, 0.13], k=min(3, failed),
        ),
        "contact_history": {"last_7d": contacts, "last_contact_hours_ago": rng.randint(2, 336)},
        "merchant_type": merchant_type,
        "customer_segment": "HIGH_VALUE" if successful >= 25 else "ESTABLISHED" if tenure >= 180 else "NEW",
    }


def _amount_minor(rng: random.Random, leak_type: str, merchant_type: str) -> int:
    base = {"SAAS": 180000, "ECOMMERCE": 90000, "EDTECH": 260000, "MEDIA": 45000}[merchant_type]
    if leak_type == "CHECKOUT_ABANDONMENT":
        base = int(base * 0.75)
    return max(9900, min(2_500_000, int(rng.lognormvariate(math.log(base), 0.75))))


def _failure_modifier(failure: str, action: str) -> float:
    table = {
        "TEMPORARY_BANK_FAILURE": {"RETRY_NOW": 0.25, "RETRY_LATER": 0.33},
        "INSUFFICIENT_FUNDS": {"RETRY_NOW": -0.08, "RETRY_LATER": 0.25, "SEND_REMINDER": 0.12},
        "PAYMENT_METHOD_INVALID": {"REQUEST_PAYMENT_METHOD_UPDATE": 0.38, "SEND_PAYMENT_LINK": 0.25},
        "MANDATE_FAILURE": {"REQUEST_PAYMENT_METHOD_UPDATE": 0.30, "SUGGEST_ALTERNATE_METHOD": 0.25},
        "HARD_DECLINE": {"RETRY_NOW": -0.18, "SUGGEST_ALTERNATE_METHOD": 0.29},
        "AMBIGUOUS_CUSTOMER_INTENT": {"SEND_REMINDER": 0.15, "WAIT_FOR_PROMISE_TO_PAY": 0.22},
        "PAYMENT_FAILURE": {"SEND_CHECKOUT_RECOVERY_LINK": 0.25, "SUGGEST_ALTERNATE_METHOD": 0.20},
        "PAYMENT_FRICTION": {"SEND_CHECKOUT_RECOVERY_LINK": 0.33, "SEND_PAYMENT_LINK": 0.22},
        "METHOD_MISMATCH": {"SUGGEST_ALTERNATE_METHOD": 0.39},
        "TEMPORARY_ABANDONMENT": {"WAIT": 0.10, "SEND_REMINDER": 0.19},
        "PRICE_HESITATION": {"RETENTION_ACTION": 0.35},
        "LOW_INTENT_ABANDONMENT": {"SEND_REMINDER": -0.10, "RETENTION_ACTION": 0.05},
    }
    return table.get(failure, {}).get(action, 0.0)


def _action_probability(hidden: dict[str, Any], observable: dict[str, Any], failure: str, action: str) -> float:
    natural = hidden["natural_recovery_probability"] * (1 - hidden["churn_intent"] * 0.7)
    fatigue = observable["contact_history"]["last_7d"] / 5
    if action == "WAIT":
        return _bounded(natural + _failure_modifier(failure, action))
    responsiveness = {
        "RETRY_NOW": hidden["retry_responsiveness"],
        "RETRY_LATER": hidden["retry_responsiveness"] + (0.12 if hidden["liquidity_pattern"] == "PAYDAY_CYCLICAL" else 0),
        "SEND_REMINDER": hidden["reminder_responsiveness"],
        "SEND_PAYMENT_LINK": hidden["payment_link_responsiveness"],
        "SEND_CHECKOUT_RECOVERY_LINK": hidden["payment_link_responsiveness"] + 0.08,
        "REQUEST_PAYMENT_METHOD_UPDATE": hidden["payment_link_responsiveness"],
        "SUGGEST_ALTERNATE_METHOD": 0.60 if observable["payment_method"] != hidden["payment_method_preference"] else 0.32,
        "WAIT_FOR_PROMISE_TO_PAY": hidden["promise_reliability"],
        "RETENTION_ACTION": 1 - hidden["churn_intent"],
    }[action]
    contact_action = action not in {"RETRY_NOW", "RETRY_LATER"}
    fatigue_penalty = hidden["contact_sensitivity"] * fatigue * (0.22 if contact_action else 0.06)
    return _bounded(0.04 + responsiveness * 0.38 + _failure_modifier(failure, action) - fatigue_penalty - hidden["churn_intent"] * 0.12)


def _potential_outcomes(seed: int, case_id: str, amount: int, hidden: dict[str, Any], observable: dict[str, Any], failure: str, actions: tuple[str, ...]) -> dict[str, Any]:
    outcomes = {}
    natural_probability = _action_probability(hidden, observable, failure, "WAIT")
    for action in actions:
        probability = _action_probability(hidden, observable, failure, action)
        recovered = _stable_uniform(seed, case_id, action, "outcome") < probability
        latency = 1 + int(_stable_uniform(seed, case_id, action, "latency") * (96 if action == "WAIT" else 48))
        cost = ACTION_COST_MINOR[action]
        fatigue_cost = 0 if action in {"WAIT", "RETRY_NOW", "RETRY_LATER"} else int(amount * hidden["contact_sensitivity"] * 0.002)
        incentive = min(3000, int(amount * 0.05)) if action == "RETENTION_ACTION" else 0
        outcomes[action] = {
            "recovery_probability": probability,
            "natural_recovery_probability": natural_probability,
            "incremental_uplift": round(probability - natural_probability, 6),
            "recovered": recovered,
            "recovered_amount_minor": amount if recovered else 0,
            "latency_hours": latency,
            "action_parameters": {"execute_after_hours": 24} if action == "RETRY_LATER" else {},
            "intervention_cost_minor": cost,
            "fatigue_cost_minor": fatigue_cost,
            "incentive_cost_minor": incentive,
            "net_recovered_minor": (amount if recovered else 0) - cost - fatigue_cost - incentive,
        }
    return outcomes


def _generate_case(config: SimulationConfig, index: int) -> dict[str, Any]:
    rng = random.Random((config.seed << 32) + index)
    case_id = _case_id(config.seed, index)
    leak_type = "FAILED_SUBSCRIPTION" if rng.random() < config.subscription_share else "CHECKOUT_ABANDONMENT"
    merchant_type = _weighted_choice(rng, config.merchant_mix)
    hidden = _hidden_customer(rng)
    observable = _observable_customer(rng, hidden, merchant_type)
    if leak_type == "FAILED_SUBSCRIPTION":
        failure = _weighted_choice(rng, config.subscription_failure_distribution)
        actions = SUBSCRIPTION_ACTIONS
    else:
        failure = _weighted_choice(rng, config.checkout_failure_distribution)
        actions = CHECKOUT_ACTIONS
    amount = _amount_minor(rng, leak_type, merchant_type)
    observable.update({
        "case_id": case_id, "leak_type": leak_type, "failure_type": failure,
        "amount_at_risk_minor": amount, "currency": "INR",
        "merchant_id": f"merchant-{merchant_type.lower()}-{rng.randint(1, 12):02d}",
        "customer_id": f"customer-{case_id[4:20]}",
        "hours_since_leak": rng.randint(1, 72),
    })
    return {
        "simulation_version": SIMULATION_VERSION,
        "feature_version": FEATURE_VERSION,
        "observable": observable,
        "_ground_truth": {
            "hidden_customer": hidden,
            "potential_outcomes": _potential_outcomes(
                config.seed, case_id, amount, hidden, observable, failure, actions
            ),
        },
    }


def _assign_splits(cases: list[dict[str, Any]], seed: int) -> dict[str, list[dict[str, Any]]]:
    indices = list(range(len(cases)))
    random.Random(seed ^ 0x5F3759DF).shuffle(indices)
    train_end = int(len(cases) * 0.70)
    validation_end = train_end + int(len(cases) * 0.15)
    partitions = {"train": indices[:train_end], "validation": indices[train_end:validation_end], "test": indices[validation_end:]}
    result: dict[str, list[dict[str, Any]]] = {}
    for split, selected in partitions.items():
        result[split] = []
        for index in selected:
            row = cases[index]
            row["split"] = split
            result[split].append(row)
    return result


def _report(config: SimulationConfig, splits: dict[str, list[dict[str, Any]]]) -> dict[str, Any]:
    all_cases = [case for values in splits.values() for case in values]
    observables = [case["observable"] for case in all_cases]
    revenue = sum(case["amount_at_risk_minor"] for case in observables)
    return {
        "simulation_version": SIMULATION_VERSION,
        "feature_version": FEATURE_VERSION,
        "seed": config.seed,
        "dataset_size": len(all_cases),
        "generated_at": datetime(2026, 1, 1, tzinfo=timezone.utc).isoformat(),
        "split_sizes": {name: len(values) for name, values in splits.items()},
        "revenue_at_risk_minor": revenue,
        "currency": "INR",
        "leak_type_distribution": dict(sorted(Counter(c["leak_type"] for c in observables).items())),
        "failure_distribution": dict(sorted(Counter(c["failure_type"] for c in observables).items())),
        "customer_segments": dict(sorted(Counter(c["customer_segment"] for c in observables).items())),
        "merchant_segments": dict(sorted(Counter(c["merchant_type"] for c in observables).items())),
        "configuration": asdict(config),
        "data_contract": {
            "optimizer_input": "observable",
            "evaluation_only": "_ground_truth",
            "warning": "Never expose _ground_truth fields to a strategy or fitted feature pipeline.",
        },
    }


def generate_dataset(config: SimulationConfig, output_dir: Path | str) -> dict[str, Any]:
    config.validate()
    target = Path(output_dir)
    target.mkdir(parents=True, exist_ok=True)
    cases = [_generate_case(config, index) for index in range(config.dataset_size)]
    splits = _assign_splits(cases, config.seed)
    for split, rows in splits.items():
        with (target / f"{split}.jsonl").open("w", encoding="utf-8", newline="\n") as handle:
            for row in rows:
                handle.write(json.dumps(row, sort_keys=True, separators=(",", ":")) + "\n")
    report = _report(config, splits)
    with (target / "dataset_report.json").open("w", encoding="utf-8", newline="\n") as handle:
        json.dump(report, handle, sort_keys=True, indent=2)
        handle.write("\n")
    return report
