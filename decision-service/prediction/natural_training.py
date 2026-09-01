from __future__ import annotations

import argparse
import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import joblib
import numpy as np
from sklearn.calibration import CalibratedClassifierCV
from sklearn.frozen import FrozenEstimator

from evaluation.environment import file_sha256, load_jsonl
from features.pipeline import FEATURE_NAMES, from_simulator
from prediction.training import calibration_error, metrics, pipeline

MODEL_VERSION = "natural-recovery-v1"
FEATURE_VERSION = "natural-features-v1"


def examples(rows: list[dict[str, Any]]) -> tuple[list[list[Any]], np.ndarray, list[dict[str, Any]]]:
    features, labels, metadata = [], [], []
    for row in rows:
        observable = row["observable"]
        outcome = row["_ground_truth"]["potential_outcomes"]["WAIT"]
        features.append(from_simulator(observable, "WAIT"))
        labels.append(int(outcome["recovered"]))
        metadata.append({"case_id": observable["case_id"], "action": "WAIT", "amount": observable["amount_at_risk_minor"], "net": outcome["net_recovered_minor"]})
    return features, np.asarray(labels, dtype=int), metadata


def train(train_path: Path, validation_path: Path, output_dir: Path, seed: int = 42) -> dict[str, Any]:
    train_rows = load_jsonl(train_path, "train")
    validation_rows = load_jsonl(validation_path, "validation")
    midpoint = len(validation_rows) // 2
    calibration_rows, selection_rows = validation_rows[:midpoint], validation_rows[midpoint:]
    x_train, y_train, _ = examples(train_rows)
    x_cal, y_cal, _ = examples(calibration_rows)
    x_select, y_select, selection_meta = examples(selection_rows)
    candidates: dict[str, tuple[Any, dict[str, Any]]] = {}
    for algorithm in ("logistic_regression", "gradient_boosting"):
        base = pipeline(algorithm, seed).fit(x_train, y_train)
        models = {"raw": base}
        for method in ("sigmoid", "isotonic"):
            models[method] = CalibratedClassifierCV(FrozenEstimator(base), method=method).fit(x_cal, y_cal)
        calibration_metrics = {method: metrics(model, x_select, y_select, selection_meta) for method, model in models.items()}
        selected_method = min(calibration_metrics, key=lambda method: (calibration_metrics[method]["brier_score"], calibration_metrics[method]["expected_calibration_error"], method))
        result = dict(calibration_metrics[selected_method])
        result["calibration_method"] = selected_method
        result["calibration_comparison"] = {method: {"brier_score": value["brier_score"], "expected_calibration_error": value["expected_calibration_error"]} for method, value in calibration_metrics.items()}
        candidates[algorithm] = (models[selected_method], result)
    chosen = min(candidates, key=lambda name: (candidates[name][1]["brier_score"], candidates[name][1]["expected_calibration_error"]))
    output_dir.mkdir(parents=True, exist_ok=True)
    artifact = output_dir / "natural_recovery_v1.joblib"
    joblib.dump({"model": candidates[chosen][0], "model_version": MODEL_VERSION,
        "feature_version": FEATURE_VERSION, "feature_names": FEATURE_NAMES, "algorithm": chosen}, artifact)
    metadata = {"model_version":MODEL_VERSION,"feature_version":FEATURE_VERSION,"algorithm":chosen,
        "candidate_metrics":{name:value for name,(_,value) in candidates.items()},"calibration":{"method":candidates[chosen][1]["calibration_method"],"methods_compared":["raw","sigmoid","isotonic"],"dataset":"first half of validation","selection_dataset":"second half of validation"},
        "training_dataset":{"path":str(train_path),"sha256":file_sha256(train_path),"rows":len(train_rows)},
        "validation_dataset":{"path":str(validation_path),"sha256":file_sha256(validation_path),"rows":len(validation_rows)},
        "held_out_test_used":False,"seed":seed,"training_timestamp":datetime.now(timezone.utc).isoformat(),"artifact":str(artifact)}
    (output_dir/"natural_recovery_v1_metadata.json").write_text(json.dumps(metadata,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    return metadata


def main()->None:
    parser=argparse.ArgumentParser();parser.add_argument("--train",type=Path,required=True);parser.add_argument("--validation",type=Path,required=True);parser.add_argument("--output-dir",type=Path,default=Path("models"));parser.add_argument("--seed",type=int,default=42);args=parser.parse_args()
    print(json.dumps(train(args.train,args.validation,args.output_dir,args.seed),indent=2,sort_keys=True))
if __name__=="__main__":main()
