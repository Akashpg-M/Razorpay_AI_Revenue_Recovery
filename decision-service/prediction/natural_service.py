from __future__ import annotations
import os
from datetime import datetime,timezone
from functools import lru_cache
from pathlib import Path
from typing import Any
import joblib
from fastapi import APIRouter,HTTPException
from pydantic import BaseModel,ConfigDict,Field
from features.pipeline import from_decision_context

class NaturalPredictionRequest(BaseModel):
    model_config=ConfigDict(extra="forbid")
    context:dict[str,Any]
class NaturalPredictionResponse(BaseModel):
    natural_recovery_probability:float=Field(ge=0,le=1)
    model_version:str
    feature_version:str
    prediction_timestamp:datetime
def default_path()->Path:return Path(os.getenv("NATURAL_MODEL_PATH",Path(__file__).parents[1]/"models"/"natural_recovery_v1.joblib"))
@lru_cache(maxsize=4)
def load(path:str)->dict[str,Any]:
    bundle=joblib.load(path)
    if bundle.get("model_version")!="natural-recovery-v1" or bundle.get("feature_version")!="natural-features-v1":raise ValueError("incompatible natural recovery artifact")
    return bundle
def predict_natural(request:NaturalPredictionRequest,path:Path|None=None,now:datetime|None=None)->NaturalPredictionResponse:
    bundle=load(str(path or default_path()));features=[from_decision_context(request.context,"WAIT")];probability=float(bundle["model"].predict_proba(features)[0,1])
    return NaturalPredictionResponse(natural_recovery_probability=probability,model_version=bundle["model_version"],feature_version=bundle["feature_version"],prediction_timestamp=now or datetime.now(timezone.utc))
router=APIRouter(prefix="/api/v1/predict",tags=["prediction"])
@router.post("/natural",response_model=NaturalPredictionResponse)
async def natural(request:NaturalPredictionRequest)->NaturalPredictionResponse:
    try:return predict_natural(request)
    except(ValueError,OSError)as error:raise HTTPException(status_code=422,detail=str(error))from error
