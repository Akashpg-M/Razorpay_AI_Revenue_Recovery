from __future__ import annotations

from collections import defaultdict
from typing import Any

from evaluation.contracts import Decision
from evaluation.environment import resolve


class NoRecovery:
    name = "no_recovery"
    version = "1.0.0"

    def decide(self, observable: dict[str, Any]) -> Decision:
        return Decision("WAIT", reason="natural_recovery_control")


class FixedRetry:
    name = "fixed_retry"
    version = "1.0.0"

    def decide(self, observable: dict[str, Any]) -> Decision:
        if observable["leak_type"] == "FAILED_SUBSCRIPTION":
            return Decision("RETRY_24H", delay_hours=24, reason="fixed_subscription_retry")
        return Decision("SEND_CHECKOUT_RECOVERY_LINK", delay_hours=24, reason="fixed_checkout_recovery_link")


class RuleBased:
    name = "rules"
    version = "rules-v1"

    SUBSCRIPTION_RULES = {
        "TEMPORARY_BANK_FAILURE": Decision("RETRY_NOW", reason="temporary_failure"),
        "INSUFFICIENT_FUNDS": Decision("RETRY_24H", 24, "liquidity_delay"),
        "PAYMENT_METHOD_INVALID": Decision("REQUEST_PAYMENT_METHOD_UPDATE", reason="invalid_method"),
        "MANDATE_FAILURE": Decision("SEND_PAYMENT_LINK", reason="mandate_unavailable"),
        "HARD_DECLINE": Decision("STOP", reason="non_recoverable_decline"),
        "AMBIGUOUS_CUSTOMER_INTENT": Decision("SEND_REMINDER", reason="clarify_intent"),
    }
    CHECKOUT_RULES = {
        "PAYMENT_FAILURE": Decision("SEND_CHECKOUT_RECOVERY_LINK", reason="failed_checkout_payment"),
        "PAYMENT_FRICTION": Decision("SEND_CHECKOUT_RECOVERY_LINK", reason="resume_checkout"),
        "METHOD_MISMATCH": Decision("SUGGEST_ALTERNATE_METHOD", reason="method_mismatch"),
        "TEMPORARY_ABANDONMENT": Decision("SEND_REMINDER", reason="temporary_abandonment"),
        "PRICE_HESITATION": Decision("RETENTION_ACTION", reason="price_hesitation"),
        "LOW_INTENT_ABANDONMENT": Decision("STOP", reason="low_intent"),
    }

    def decide(self, observable: dict[str, Any]) -> Decision:
        rules = self.SUBSCRIPTION_RULES if observable["leak_type"] == "FAILED_SUBSCRIPTION" else self.CHECKOUT_RULES
        return rules.get(observable["failure_type"], Decision("STOP", reason="unsupported_failure"))


class ContextualRetry:
    name = "contextual_retry"
    version = "empirical-retry-v1"
    ALLOWED = {"RETRY_NOW", "RETRY_6H", "RETRY_24H", "RETRY_48H", "WAIT", "STOP"}
    CANDIDATES = (Decision("RETRY_NOW"), Decision("RETRY_6H", 6), Decision("RETRY_24H", 24), Decision("RETRY_48H", 48), Decision("WAIT"))

    def __init__(self) -> None:
        self._group_rates: dict[tuple[str, str, str, str], dict[str, float]] = {}
        self._global_rates: dict[str, float] = {}
        self._fitted = False

    @staticmethod
    def _group(observable: dict[str, Any]) -> tuple[str, str, str]:
        fatigue = "HIGH" if observable["contact_history"]["last_7d"] >= 3 else "LOW"
        return observable["failure_type"], observable["payment_method"], fatigue

    def fit(self, rows: list[dict[str, Any]], seed: int) -> "ContextualRetry":
        if not rows or any(row.get("split") != "train" for row in rows):
            raise ValueError("contextual retry may be fitted only on the training split")
        sums: dict[tuple[str, str, str, str], list[int]] = defaultdict(list)
        global_sums: dict[str, list[int]] = defaultdict(list)
        for row in rows:
            observable = row["observable"]
            if observable["leak_type"] != "FAILED_SUBSCRIPTION":
                continue
            group = self._group(observable)
            for candidate in self.CANDIDATES:
                recovered = int(resolve(row, candidate, seed).recovered)
                sums[(*group, candidate.action)].append(recovered)
                global_sums[candidate.action].append(recovered)
        self._group_rates = {key: {"rate": (sum(values) + 2) / (len(values) + 4), "n": len(values)} for key, values in sums.items()}
        self._global_rates = {action: (sum(values) + 2) / (len(values) + 4) for action, values in global_sums.items()}
        self._fitted = True
        return self

    def decide(self, observable: dict[str, Any]) -> Decision:
        if not self._fitted:
            raise RuntimeError("contextual retry strategy is not fitted")
        if observable["leak_type"] != "FAILED_SUBSCRIPTION":
            return Decision("WAIT", reason="retry_only_subscription_scope")
        if observable["failure_type"] in {"HARD_DECLINE", "PAYMENT_METHOD_INVALID", "MANDATE_FAILURE"}:
            return Decision("STOP", reason="same_method_retry_not_meaningful")
        group = self._group(observable)
        amount = observable["amount_at_risk_minor"]
        fatigue_penalty = observable["contact_history"]["last_7d"] * 0.002
        scored: list[tuple[float, Decision]] = []
        for candidate in self.CANDIDATES:
            group_value = self._group_rates.get((*group, candidate.action))
            rate = group_value["rate"] if group_value and group_value["n"] >= 8 else self._global_rates[candidate.action]
            cost = 0 if candidate.action == "WAIT" else 35
            score = rate * amount - cost - amount * fatigue_penalty * (candidate.action != "WAIT")
            scored.append((score, candidate))
        best = max(scored, key=lambda item: (item[0], -self.CANDIDATES.index(item[1])))[1]
        return Decision(best.action, best.delay_hours, "train_fitted_contextual_expected_value")
