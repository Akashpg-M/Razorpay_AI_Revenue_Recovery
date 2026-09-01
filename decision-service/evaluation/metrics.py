from __future__ import annotations

from collections import defaultdict
from typing import Any

from evaluation.contracts import MetricAccumulator, Strategy
from evaluation.environment import resolve


def _summarize(acc: MetricAccumulator) -> dict[str, Any]:
    interventions = acc.attempts + acc.contacts
    return {
        "cases": acc.cases,
        "total_revenue_at_risk_minor": acc.revenue_at_risk_minor,
        "total_revenue_recovered_minor": acc.recovered_minor,
        "recovery_rate": round(acc.recovered_cases / max(1, acc.cases), 6),
        "recovery_attempts": acc.attempts,
        "customer_contacts": acc.contacts,
        "average_recovery_latency_hours": round(sum(acc.latency_hours) / max(1, len(acc.latency_hours)), 4),
        "natural_recovery_count": acc.natural_recovery_cases,
        "natural_recovery_value_minor": acc.natural_recovery_minor,
        "intervention_cost_minor": acc.intervention_cost_minor,
        "revenue_per_intervention_minor": round(acc.recovered_minor / interventions, 2) if interventions else 0,
        "recovered_revenue_by_failure_minor": dict(sorted(acc.by_failure.items())),
        "recovered_revenue_by_action_minor": dict(sorted(acc.by_action.items())),
    }


def evaluate(rows: list[dict[str, Any]], strategy: Strategy, seed: int) -> dict[str, Any]:
    accumulators = {"aggregate": MetricAccumulator(), "subscription": MetricAccumulator(), "checkout": MetricAccumulator()}
    action_counts: dict[str, int] = defaultdict(int)
    for row in rows:
        observable = row["observable"]
        decision = strategy.decide(observable)
        outcome = resolve(row, decision, seed)
        vertical = "subscription" if observable["leak_type"] == "FAILED_SUBSCRIPTION" else "checkout"
        action_counts[decision.action] += 1
        for key in ("aggregate", vertical):
            acc = accumulators[key]
            acc.cases += 1
            acc.revenue_at_risk_minor += observable["amount_at_risk_minor"]
            acc.attempts += int(outcome.is_attempt)
            acc.contacts += int(outcome.is_contact)
            acc.intervention_cost_minor += outcome.intervention_cost_minor
            if outcome.recovered:
                acc.recovered_cases += 1
                acc.recovered_minor += outcome.recovered_amount_minor
                acc.latency_hours.append(outcome.latency_hours)
                acc.by_failure[observable["failure_type"]] = acc.by_failure.get(observable["failure_type"], 0) + outcome.recovered_amount_minor
                acc.by_action[decision.action] = acc.by_action.get(decision.action, 0) + outcome.recovered_amount_minor
            if outcome.natural_recovery:
                acc.natural_recovery_cases += 1
                acc.natural_recovery_minor += outcome.recovered_amount_minor
    return {
        "baseline": strategy.name,
        "baseline_version": strategy.version,
        "seed": seed,
        "metrics": {key: _summarize(value) for key, value in accumulators.items()},
        "action_counts": dict(sorted(action_counts.items())),
    }
