from __future__ import annotations

from dataclasses import dataclass
from typing import Any

STATUSES = {"CANDIDATE", "APPROVED", "ACTIVE", "RETIRED", "REJECTED"}
TRANSITIONS = {"CANDIDATE": {"APPROVED", "REJECTED"}, "APPROVED": {"ACTIVE", "REJECTED"},
               "ACTIVE": {"RETIRED"}, "RETIRED": set(), "REJECTED": set()}


@dataclass(frozen=True)
class ModelEntry:
    model_version: str
    model_type: str
    feature_version: str
    training_dataset_version: str
    algorithm: str
    validation_metrics: dict[str, Any]
    calibration_metrics: dict[str, Any]
    artifact_uri: str
    artifact_hash: str


def validate_candidate(entry: ModelEntry) -> None:
    required = (entry.model_version, entry.model_type, entry.feature_version, entry.training_dataset_version,
                entry.algorithm, entry.artifact_uri, entry.artifact_hash)
    if not all(required):
        raise ValueError("model candidate metadata is incomplete")
    if "brier_score" not in entry.calibration_metrics:
        raise ValueError("calibration metrics must include brier_score")


def transition(current: str, target: str, *, actor: str, reason: str) -> dict[str, str]:
    """Build an explicit append-only status event; training never invokes this automatically."""
    if target not in STATUSES or target not in TRANSITIONS.get(current, set()):
        raise ValueError(f"invalid model status transition {current} -> {target}")
    if not actor or not reason:
        raise ValueError("actor and reason are required for model status changes")
    return {"from_status": current, "status": target, "actor": actor, "reason": reason}
