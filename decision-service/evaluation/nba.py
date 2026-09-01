from __future__ import annotations

import argparse
import json
from collections import Counter
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import joblib

from evaluation.contracts import Decision
from evaluation.environment import CONTACT_ACTIONS, file_sha256, load_jsonl
from evaluation.metrics import evaluate
from features.pipeline import from_simulator
from simulation.generator import CHECKOUT_ACTIONS, SUBSCRIPTION_ACTIONS

OPTIMIZER_VERSION = "nba-v1"
COST_MODEL_VERSION = "cost-v1"
ECONOMIC_GATE_VERSION = "economic-gate-v1"
POLICY_VERSION = "policy-v1"
ACTION_COSTS = {
    "RETRY_NOW": (0, 35), "RETRY_LATER": (0, 35), "SEND_REMINDER": (25, 0),
    "SEND_PAYMENT_LINK": (25, 45), "SEND_CHECKOUT_RECOVERY_LINK": (25, 45),
    "REQUEST_PAYMENT_METHOD_UPDATE": (25, 35), "SUGGEST_ALTERNATE_METHOD": (25, 30),
    "WAIT_FOR_PROMISE_TO_PAY": (0, 10), "RETENTION_ACTION": (25, 100), "WAIT": (0, 0),
}


def _minor_probability(amount: int, probability: float) -> int:
    micros = round(probability * 1_000_000)
    product = amount * micros
    return product // 1_000_000 if product >= 0 else -(abs(product) // 1_000_000)


def rank_candidates(observable: dict[str, Any], action_probabilities: dict[str, float], natural_probability: float) -> list[dict[str, Any]]:
    amount = int(observable["amount_at_risk_minor"])
    fatigue = min(1.0, float((observable.get("contact_history") or {}).get("last_7d", 0)) / 5.0)
    failure = observable.get("failure_type", "")
    confidence = .94 if failure == "HARD_DECLINE" else (.9 if failure in {"TEMPORARY_BANK_FAILURE", "INSUFFICIENT_FUNDS", "PAYMENT_FAILURE", "PAYMENT_FRICTION", "TEMPORARY_ABANDONMENT"} else .86 if failure in {"PAYMENT_METHOD_INVALID", "MANDATE_FAILURE", "METHOD_MISMATCH", "PRICE_HESITATION"} else .5)
    candidates = []
    probabilities = dict(action_probabilities)
    probabilities["WAIT"] = natural_probability
    for action, probability in probabilities.items():
        uplift = probability - natural_probability
        gross = _minor_probability(amount, uplift)
        channel, operations = ACTION_COSTS[action]
        fatigue_penalty = amount * round(fatigue * 80) // 10_000 if action in CONTACT_ACTIONS else 0
        risk_penalty = 0 if action == "WAIT" else amount * round((1 - confidence) * 50) // 10_000
        incentive = amount * 500 // 10_000 if action == "RETENTION_ACTION" else 0
        nerv = gross - channel - operations - fatigue_penalty - risk_penalty - incentive
        candidates.append({"action": action, "action_probability": probability, "natural_probability": natural_probability,
                           "incremental_uplift": uplift, "gross_incremental_value_minor": gross,
                           "channel_cost_minor": channel, "operational_cost_minor": operations,
                           "fatigue_penalty_minor": fatigue_penalty, "risk_penalty_minor": risk_penalty,
                           "incentive_cost_minor": incentive, "nerv_minor": nerv})
    return sorted(candidates, key=lambda item: (-item["nerv_minor"], item["action"]))


def eligible_actions(observable: dict[str, Any]) -> tuple[str, ...]:
    actions = SUBSCRIPTION_ACTIONS if observable["leak_type"] == "FAILED_SUBSCRIPTION" else CHECKOUT_ACTIONS
    contacts = int((observable.get("contact_history") or {}).get("last_7d", 0))
    retries = sum(str(item).startswith("RETRY") for item in observable.get("past_recovery_actions", []))
    return tuple(action for action in actions if action != "WAIT" and not (action in CONTACT_ACTIONS and contacts >= 5) and not (action.startswith("RETRY") and retries >= 3))


@dataclass
class FrozenNBA:
    decisions: dict[str, Decision]
    name: str = "learned_nba_intermediate"
    version: str = OPTIMIZER_VERSION
    def decide(self, observable: dict[str, Any]) -> Decision:
        return self.decisions[observable["case_id"]]


def run(dataset: Path, outcome_artifact: Path, natural_artifact: Path, baseline_path: Path, output: Path, seed: int = 42) -> dict[str, Any]:
    rows = load_jsonl(dataset, "test")
    outcome_bundle, natural_bundle = joblib.load(outcome_artifact), joblib.load(natural_artifact)
    natural_features = [from_simulator(row["observable"], "WAIT") for row in rows]
    natural_probabilities = natural_bundle["model"].predict_proba(natural_features)[:, 1]

    feature_rows, coordinates = [], []
    for row_index, row in enumerate(rows):
        for action in eligible_actions(row["observable"]):
            feature_rows.append(from_simulator(row["observable"], action)); coordinates.append((row_index, action))
    predicted = outcome_bundle["model"].predict_proba(feature_rows)[:, 1]
    by_case: list[dict[str, float]] = [{} for _ in rows]
    for (row_index, action), probability in zip(coordinates, predicted): by_case[row_index][action] = float(probability)

    decisions, action_counts = {}, Counter()
    negative_candidates = negative_avoided = waits = economic_blocks = policy_denials = 0
    selected_nerv_total = 0
    for index, row in enumerate(rows):
        ranked = rank_candidates(row["observable"], by_case[index], float(natural_probabilities[index]))
        selected = ranked[0]
        negative_candidates += sum(item["incremental_uplift"] < 0 for item in ranked if item["action"] != "WAIT")
        negative_avoided += sum(item["incremental_uplift"] < 0 and item["action"] != selected["action"] for item in ranked)
        if selected["action"] != "WAIT" and selected["nerv_minor"] < 0:
            economic_blocks += 1; selected = next(item for item in ranked if item["action"] == "WAIT")
        action = selected["action"]
        waits += action == "WAIT"; selected_nerv_total += selected["nerv_minor"]; action_counts[action] += 1
        decisions[row["observable"]["case_id"]] = Decision(action=action, reason="maximum deterministic NERV")

    result = evaluate(rows, FrozenNBA(decisions), seed)
    for metrics in result["metrics"].values():
        metrics["net_recovered_value_minor"] = metrics["total_revenue_recovered_minor"] - metrics["intervention_cost_minor"]
    result.update({
        "dataset_path": str(dataset), "dataset_sha256": file_sha256(dataset), "dataset_size": len(rows),
        "split": "test", "simulation_version": rows[0]["simulation_version"],
        "versions": {"optimizer": OPTIMIZER_VERSION, "cost_model": COST_MODEL_VERSION, "economic_gate": ECONOMIC_GATE_VERSION,
                     "policy": POLICY_VERSION, "outcome_model": outcome_bundle["model_version"], "natural_model": natural_bundle["model_version"]},
        "decision_diagnostics": {"wait_selections": waits, "economic_blocks": economic_blocks, "policy_denials": policy_denials, "policy_escalations": 0,
                                 "negative_uplift_candidates": negative_candidates, "negative_uplift_candidates_avoided": negative_avoided,
                                 "selected_predicted_nerv_minor": selected_nerv_total, "action_counts": dict(sorted(action_counts.items()))},
        "evaluation_limitations": ["Synthetic potential outcomes are used only for held-out evaluation.",
                                   "This is intermediate policy evaluation, not causal production attribution.",
                                   "Merchant thresholds and channel availability are not present in the synthetic observable schema."],
        "incremental_attributed_recovered_value_minor": None,
    })
    baselines = json.loads(baseline_path.read_text(encoding="utf-8"))["baselines"]
    result["baseline_comparison"] = [{"baseline": b["baseline"], "recovered_minor": b["metrics"]["aggregate"]["total_revenue_recovered_minor"],
                                      "interventions": b["metrics"]["aggregate"]["recovery_attempts"] + b["metrics"]["aggregate"]["customer_contacts"]} for b in baselines]
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    markdown = output.with_suffix(".md")
    aggregate = result["metrics"]["aggregate"]
    lines = ["# Intermediate NBA held-out evaluation", "", f"Dataset SHA-256: `{result['dataset_sha256']}`", "",
             "| Strategy | Recovered (minor) | Interventions |", "|---|---:|---:|",
             f"| learned_nba_intermediate | {aggregate['total_revenue_recovered_minor']} | {aggregate['recovery_attempts'] + aggregate['customer_contacts']} |"]
    lines += [f"| {item['baseline']} | {item['recovered_minor']} | {item['interventions']} |" for item in result["baseline_comparison"]]
    lines += ["", "This result is synthetic and intermediate; it is not a production causal-attribution claim.", ""]
    markdown.write_text("\n".join(lines), encoding="utf-8")
    return result


def main() -> None:
    parser = argparse.ArgumentParser(); parser.add_argument("--dataset", type=Path, required=True); parser.add_argument("--outcome-artifact", type=Path, required=True); parser.add_argument("--natural-artifact", type=Path, required=True); parser.add_argument("--baselines", type=Path, required=True); parser.add_argument("--output", type=Path, required=True); parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args(); print(json.dumps(run(args.dataset, args.outcome_artifact, args.natural_artifact, args.baselines, args.output, args.seed), indent=2, sort_keys=True))

if __name__ == "__main__": main()
