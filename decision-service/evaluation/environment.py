from __future__ import annotations

import hashlib
import json
from pathlib import Path
from typing import Any, Iterable

from evaluation.contracts import Decision, ResolvedOutcome


CONTACT_ACTIONS = {
    "SEND_REMINDER", "SEND_PAYMENT_LINK", "SEND_CHECKOUT_RECOVERY_LINK",
    "REQUEST_PAYMENT_METHOD_UPDATE", "SUGGEST_ALTERNATE_METHOD",
    "WAIT_FOR_PROMISE_TO_PAY", "RETENTION_ACTION",
}
ATTEMPT_ACTIONS = {"RETRY_NOW", "RETRY_LATER", "RETRY_6H", "RETRY_24H", "RETRY_48H"}


def file_sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest().upper()


def load_jsonl(path: Path, required_split: str | None = None) -> list[dict[str, Any]]:
    rows = [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line]
    if required_split and any(row.get("split") != required_split for row in rows):
        raise ValueError(f"{path} contains rows outside the required {required_split!r} split")
    return rows


def _stable_uniform(seed: int, case_id: str, action: str) -> float:
    digest = hashlib.sha256(f"baseline:{seed}:{case_id}:{action}".encode()).digest()
    return int.from_bytes(digest[:8], "big") / 2**64


def _retry_variant_outcome(row: dict[str, Any], decision: Decision, seed: int) -> dict[str, Any]:
    truth = row["_ground_truth"]
    base = dict(truth["potential_outcomes"]["RETRY_LATER"])
    delay = decision.delay_hours or {"RETRY_6H": 6, "RETRY_24H": 24, "RETRY_48H": 48}.get(decision.action, 24)
    hidden = truth["hidden_customer"]
    probability = float(base["recovery_probability"])
    if delay == 6:
        probability += 0.06 if hidden["liquidity_pattern"] == "STABLE" else -0.05
    elif delay == 48:
        probability += 0.08 if hidden["liquidity_pattern"] in {"PAYDAY_CYCLICAL", "VOLATILE"} else -0.03
    probability = max(0.005, min(0.95, probability))
    variant = f"RETRY_LATER:{delay}H"
    recovered = _stable_uniform(seed, row["observable"]["case_id"], variant) < probability
    amount = row["observable"]["amount_at_risk_minor"]
    base.update({
        "recovery_probability": round(probability, 6),
        "recovered": recovered,
        "recovered_amount_minor": amount if recovered else 0,
        "net_recovered_minor": (amount if recovered else 0) - int(base["intervention_cost_minor"]),
        "latency_hours": delay + int(_stable_uniform(seed, row["observable"]["case_id"], variant + ":latency") * 12),
    })
    return base


def resolve(row: dict[str, Any], decision: Decision, seed: int) -> ResolvedOutcome:
    observable = row["observable"]
    if decision.action == "STOP":
        outcome = {"recovered": False, "recovered_amount_minor": 0, "latency_hours": 0, "intervention_cost_minor": 0}
    elif decision.action in {"RETRY_6H", "RETRY_24H", "RETRY_48H"}:
        outcome = _retry_variant_outcome(row, decision, seed)
    else:
        try:
            outcome = row["_ground_truth"]["potential_outcomes"][decision.canonical_action]
        except KeyError as error:
            raise ValueError(f"dataset has no outcome for {decision.action}") from error
    canonical = decision.canonical_action
    natural = canonical == "WAIT" and bool(outcome["recovered"])
    return ResolvedOutcome(
        recovered=bool(outcome["recovered"]),
        recovered_amount_minor=int(outcome["recovered_amount_minor"]),
        latency_hours=int(outcome["latency_hours"]),
        intervention_cost_minor=int(outcome["intervention_cost_minor"]),
        is_contact=canonical in CONTACT_ACTIONS,
        is_attempt=decision.action in ATTEMPT_ACTIONS or canonical in ATTEMPT_ACTIONS,
        natural_recovery=natural,
    )


def observable_only(rows: Iterable[dict[str, Any]]) -> Iterable[dict[str, Any]]:
    for row in rows:
        yield row["observable"]
