from __future__ import annotations

import os
from datetime import datetime, timezone
from functools import lru_cache
from pathlib import Path
from typing import Any

import joblib
from fastapi import APIRouter, HTTPException
from pydantic import BaseModel, ConfigDict, Field, field_validator

from features.pipeline import FEATURE_VERSION, from_decision_context


CANONICAL_ACTIONS = {
    "WAIT", "RETRY_NOW", "RETRY_LATER", "SEND_REMINDER", "SEND_PAYMENT_LINK",
    "SEND_CHECKOUT_RECOVERY_LINK", "REQUEST_PAYMENT_METHOD_UPDATE",
    "SUGGEST_ALTERNATE_METHOD", "WAIT_FOR_PROMISE_TO_PAY", "RETENTION_ACTION",
    "ESCALATE_TO_HUMAN", "STOP",
}


class PredictionRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")
    context: dict[str, Any]
    eligible_actions: list[str] = Field(min_length=1)

    @field_validator("eligible_actions")
    @classmethod
    def actions_are_closed_and_unique(cls, values: list[str]) -> list[str]:
        if len(values) != len(set(values)):
            raise ValueError("eligible_actions must be unique")
        unsupported = set(values) - CANONICAL_ACTIONS
        if unsupported:
            raise ValueError(f"unsupported actions: {sorted(unsupported)}")
        return values


class OutcomePrediction(BaseModel):
    action: str
    recovery_probability: float = Field(ge=0, le=1)
    model_version: str
    feature_version: str
    prediction_timestamp: datetime


class PredictionResponse(BaseModel):
    predictions: list[OutcomePrediction]


def default_model_path() -> Path:
    return Path(os.getenv("OUTCOME_MODEL_PATH", Path(__file__).parents[1] / "models" / "outcome_v1.joblib"))


@lru_cache(maxsize=4)
def load_bundle(path: str) -> dict[str, Any]:
    bundle = joblib.load(path)
    required = {"model", "model_version", "feature_version", "feature_names", "algorithm"}
    if not required <= bundle.keys():
        raise ValueError("model artifact is missing required metadata")
    if bundle["feature_version"] != FEATURE_VERSION:
        raise ValueError("model artifact feature version is incompatible")
    return bundle


def predict(request: PredictionRequest, path: Path | None = None, now: datetime | None = None) -> PredictionResponse:
    if request.context.get("feature_version") not in {FEATURE_VERSION, "recovery-context-v1"}:
        raise ValueError("unsupported decision context feature_version")
    bundle = load_bundle(str(path or default_model_path()))
    features = [from_decision_context(request.context, action) for action in request.eligible_actions]
    probabilities = bundle["model"].predict_proba(features)[:, 1]
    timestamp = now or datetime.now(timezone.utc)
    return PredictionResponse(predictions=[
        OutcomePrediction(action=action, recovery_probability=float(probability),
            model_version=bundle["model_version"], feature_version=bundle["feature_version"],
            prediction_timestamp=timestamp)
        for action, probability in zip(request.eligible_actions, probabilities)
    ])


router = APIRouter(prefix="/api/v1/predict", tags=["prediction"])


@router.post("/outcomes", response_model=PredictionResponse)
async def predict_outcomes(request: PredictionRequest) -> PredictionResponse:
    try:
        return predict(request)
    except (ValueError, OSError) as error:
        raise HTTPException(status_code=422, detail=str(error)) from error
