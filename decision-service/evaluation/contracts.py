from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Protocol


@dataclass(frozen=True)
class Decision:
    action: str
    delay_hours: int | None = None
    reason: str = ""

    @property
    def canonical_action(self) -> str:
        return "RETRY_LATER" if self.action in {"RETRY_6H", "RETRY_24H", "RETRY_48H"} else self.action


@dataclass(frozen=True)
class ResolvedOutcome:
    recovered: bool
    recovered_amount_minor: int
    latency_hours: int
    intervention_cost_minor: int
    is_contact: bool
    is_attempt: bool
    natural_recovery: bool


class Strategy(Protocol):
    name: str
    version: str

    def decide(self, observable: dict[str, Any]) -> Decision: ...


@dataclass
class MetricAccumulator:
    cases: int = 0
    revenue_at_risk_minor: int = 0
    recovered_minor: int = 0
    recovered_cases: int = 0
    attempts: int = 0
    contacts: int = 0
    latency_hours: list[int] = field(default_factory=list)
    natural_recovery_cases: int = 0
    natural_recovery_minor: int = 0
    intervention_cost_minor: int = 0
    by_failure: dict[str, int] = field(default_factory=dict)
    by_action: dict[str, int] = field(default_factory=dict)
