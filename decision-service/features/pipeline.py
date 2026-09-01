from __future__ import annotations

from typing import Any

FEATURE_VERSION = "features-v1"

CATEGORICAL_FEATURES = (
    "leak_type", "failure_type", "payment_method", "previous_action",
    "customer_segment", "merchant_type", "checkout_stage", "action",
)
NUMERIC_FEATURES = (
    "amount_minor", "subscription_tenure_days", "previous_failures",
    "successful_payments", "successful_payment_ratio", "retry_count",
    "contact_count", "hours_since_failure", "fatigue", "promise_reliability",
)
FEATURE_NAMES = CATEGORICAL_FEATURES + NUMERIC_FEATURES
HIDDEN_FEATURES = {
    "liquidity_pattern", "retry_responsiveness", "payment_link_responsiveness",
    "reminder_responsiveness", "contact_sensitivity", "natural_recovery_probability",
    "payment_method_preference", "churn_intent",
}

FAILURE_ALIASES = {
    "AMBIGUOUS_CUSTOMER_INTENT": "CUSTOMER_INTENT_OR_UNKNOWN",
    "METHOD_MISMATCH": "PAYMENT_METHOD_MISMATCH",
    "TEMPORARY_ABANDONMENT": "DELAYED_INTENT",
    "PRICE_HESITATION": "PRICE_OR_VALUE_HESITATION",
    "LOW_INTENT_ABANDONMENT": "UNKNOWN_ABANDONMENT",
}


def from_simulator(observable: dict[str, Any], action: str) -> list[Any]:
    _reject_hidden(observable)
    history = observable.get("payment_history") or {}
    actions = observable.get("past_recovery_actions") or []
    contacts = observable.get("contact_history") or {}
    record = {
        "leak_type": observable.get("leak_type", "UNKNOWN"),
        "failure_type": normalize_failure(observable.get("failure_type", "UNKNOWN")),
        "payment_method": observable.get("payment_method", "UNKNOWN"),
        "previous_action": actions[-1] if actions else "NONE",
        "customer_segment": observable.get("customer_segment", "UNKNOWN"),
        "merchant_type": observable.get("merchant_type", "UNKNOWN"),
        "checkout_stage": observable.get("checkout_stage", "UNKNOWN"),
        "action": action,
        "amount_minor": observable.get("amount_at_risk_minor", 0),
        "subscription_tenure_days": observable.get("subscription_tenure_days", 0),
        "previous_failures": observable.get("failure_count_90d", history.get("failed_count", 0)),
        "successful_payments": history.get("successful_count", 0),
        "successful_payment_ratio": history.get("success_rate", 0.0),
        "retry_count": sum(str(item).startswith("RETRY") for item in actions),
        "contact_count": contacts.get("last_7d", 0),
        "hours_since_failure": observable.get("hours_since_leak", 0),
        "fatigue": min(1.0, float(contacts.get("last_7d", 0)) / 5.0),
        "promise_reliability": observable.get("promise_reliability", 0.5),
    }
    return [record[name] for name in FEATURE_NAMES]


def from_decision_context(context: dict[str, Any], action: str) -> list[Any]:
    _reject_hidden(context)
    case = context.get("case") or {}
    diagnosis = context.get("diagnosis") or {}
    customer = context.get("customer_profile") or {}
    merchant = context.get("merchant_context") or {}
    timing = context.get("timing_context") or {}
    payment = context.get("payment_state") or {}
    actions = context.get("recent_actions") or []
    failure_context = case.get("failure_or_leak_context") or {}
    if isinstance(failure_context, str):
        failure_context = {}
    record = {
        "leak_type": case.get("leak_type", "UNKNOWN"),
        "failure_type": normalize_failure(diagnosis.get("failure_category", "UNKNOWN")),
        "payment_method": customer.get("preferred_payment_method") or failure_context.get("payment_method", "UNKNOWN"),
        "previous_action": actions[0].get("action", "NONE") if actions else "NONE",
        "customer_segment": failure_context.get("customer_segment", "UNKNOWN"),
        "merchant_type": merchant.get("merchant_type", "UNKNOWN"),
        "checkout_stage": failure_context.get("checkout_stage", "UNKNOWN"),
        "action": action,
        "amount_minor": case.get("amount_at_risk_minor", 0),
        "subscription_tenure_days": customer.get("subscription_tenure_days", 0),
        "previous_failures": customer.get("recent_failures", 0),
        "successful_payments": customer.get("successful_payment_count", 0),
        "successful_payment_ratio": customer.get("successful_payment_ratio", 0.0),
        "retry_count": sum(str(item.get("action", "")).startswith("RETRY") for item in actions),
        "contact_count": customer.get("contact_count", 0),
        "hours_since_failure": timing.get("hours_since_leak", 0),
        "fatigue": customer.get("recovery_fatigue", 0.0),
        "promise_reliability": customer.get("promise_reliability", 0.5),
    }
    return [record[name] for name in FEATURE_NAMES]


def _reject_hidden(value: Any) -> None:
    if isinstance(value, dict):
        for key, child in value.items():
            if key in HIDDEN_FEATURES:
                raise ValueError(f"hidden simulator feature {key!r} is prohibited")
            _reject_hidden(child)
    elif isinstance(value, list):
        for child in value:
            _reject_hidden(child)


def normalize_failure(value: str) -> str:
    return FAILURE_ALIASES.get(value, value)
