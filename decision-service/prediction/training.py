from __future__ import annotations

import argparse
import json
from collections import defaultdict
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import joblib
import numpy as np
from sklearn.calibration import CalibratedClassifierCV
from sklearn.compose import ColumnTransformer
from sklearn.ensemble import GradientBoostingClassifier
from sklearn.frozen import FrozenEstimator
from sklearn.impute import SimpleImputer
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score, brier_score_loss, precision_score, recall_score, roc_auc_score
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import OneHotEncoder, StandardScaler

from evaluation.environment import file_sha256, load_jsonl
from features.pipeline import CATEGORICAL_FEATURES, FEATURE_NAMES, FEATURE_VERSION, NUMERIC_FEATURES, from_simulator

MODEL_VERSION = "outcome-v1"


def examples(rows: list[dict[str, Any]]) -> tuple[list[list[Any]], np.ndarray, list[dict[str, Any]]]:
    features: list[list[Any]] = []
    labels: list[int] = []
    metadata: list[dict[str, Any]] = []
    for row in rows:
        observable = row["observable"]
        for action, outcome in row["_ground_truth"]["potential_outcomes"].items():
            features.append(from_simulator(observable, action))
            labels.append(int(outcome["recovered"]))
            metadata.append({"case_id": observable["case_id"], "action": action, "amount": observable["amount_at_risk_minor"], "net": outcome["net_recovered_minor"]})
    return features, np.asarray(labels, dtype=int), metadata


def pipeline(algorithm: str, seed: int) -> Pipeline:
    categorical_indices = list(range(len(CATEGORICAL_FEATURES)))
    numeric_indices = list(range(len(CATEGORICAL_FEATURES), len(FEATURE_NAMES)))
    preprocessing = ColumnTransformer([
        ("categorical", Pipeline([("missing", SimpleImputer(strategy="most_frequent")), ("onehot", OneHotEncoder(handle_unknown="ignore", sparse_output=False))]), categorical_indices),
        ("numeric", Pipeline([("missing", SimpleImputer(strategy="median")), ("scale", StandardScaler())]), numeric_indices),
    ])
    if algorithm == "logistic_regression":
        estimator = LogisticRegression(max_iter=600, random_state=seed)
    elif algorithm == "gradient_boosting":
        estimator = GradientBoostingClassifier(n_estimators=120, learning_rate=0.05, max_depth=3, random_state=seed)
    else:
        raise ValueError(f"unknown algorithm {algorithm}")
    return Pipeline([("preprocessing", preprocessing), ("classifier", estimator)])


def calibration_error(y_true: np.ndarray, probability: np.ndarray, bins: int = 10) -> float:
    edges = np.linspace(0, 1, bins + 1)
    result = 0.0
    for lower, upper in zip(edges[:-1], edges[1:]):
        selected = (probability >= lower) & (probability < upper if upper < 1 else probability <= upper)
        if selected.any():
            result += selected.mean() * abs(float(y_true[selected].mean()) - float(probability[selected].mean()))
    return float(result)


def metrics(model: Any, x: list[list[Any]], y: np.ndarray, metadata: list[dict[str, Any]]) -> dict[str, Any]:
    probability = model.predict_proba(x)[:, 1]
    predicted = probability >= 0.5
    by_action: dict[str, dict[str, float]] = {}
    actions = sorted({item["action"] for item in metadata})
    for action in actions:
        mask = np.asarray([item["action"] == action for item in metadata])
        if len(set(y[mask])) > 1:
            by_action[action] = {"roc_auc": round(float(roc_auc_score(y[mask], probability[mask])), 6), "brier_score": round(float(brier_score_loss(y[mask], probability[mask])), 6)}
    grouped: dict[str, list[int]] = defaultdict(list)
    for index, item in enumerate(metadata): grouped[item["case_id"]].append(index)
    realized_net = 0
    for indices in grouped.values():
        best = max(indices, key=lambda index: probability[index] * metadata[index]["amount"])
        realized_net += metadata[best]["net"]
    return {
        "roc_auc": round(float(roc_auc_score(y, probability)), 6),
        "brier_score": round(float(brier_score_loss(y, probability)), 6),
        "expected_calibration_error": round(calibration_error(y, probability), 6),
        "average_precision": round(float(average_precision_score(y, probability)), 6),
        "precision_at_0_5": round(float(precision_score(y, predicted, zero_division=0)), 6),
        "recall_at_0_5": round(float(recall_score(y, predicted, zero_division=0)), 6),
        "simple_ranker_realized_net_minor": int(realized_net),
        "by_action": by_action,
    }


def train(train_path: Path, validation_path: Path, output_dir: Path, seed: int = 42) -> dict[str, Any]:
    train_rows = load_jsonl(train_path, "train")
    validation_rows = load_jsonl(validation_path, "validation")
    midpoint = len(validation_rows) // 2
    calibration_rows, selection_rows = validation_rows[:midpoint], validation_rows[midpoint:]
    x_train, y_train, _ = examples(train_rows)
    x_calibration, y_calibration, _ = examples(calibration_rows)
    x_selection, y_selection, selection_meta = examples(selection_rows)
    candidates: dict[str, tuple[Any, dict[str, Any]]] = {}
    for algorithm in ("logistic_regression", "gradient_boosting"):
        base = pipeline(algorithm, seed).fit(x_train, y_train)
        models = {"raw": base}
        for method in ("sigmoid", "isotonic"):
            models[method] = CalibratedClassifierCV(FrozenEstimator(base), method=method).fit(x_calibration, y_calibration)
        calibration_metrics = {method: metrics(model, x_selection, y_selection, selection_meta) for method, model in models.items()}
        selected_method = min(calibration_metrics, key=lambda method: (calibration_metrics[method]["brier_score"], calibration_metrics[method]["expected_calibration_error"], method))
        result = dict(calibration_metrics[selected_method])
        result["calibration_method"] = selected_method
        result["calibration_comparison"] = {method: {"brier_score": value["brier_score"], "expected_calibration_error": value["expected_calibration_error"]} for method, value in calibration_metrics.items()}
        preprocessing = base.named_steps["preprocessing"]
        transformed_names = [str(name) for name in preprocessing.get_feature_names_out()]
        classifier = base.named_steps["classifier"]
        weights = classifier.coef_[0] if algorithm == "logistic_regression" else classifier.feature_importances_
        ranked = sorted(zip(transformed_names, weights), key=lambda item: abs(float(item[1])), reverse=True)[:30]
        result["top_features"] = [{"feature": name, "weight": round(float(weight), 8)} for name, weight in ranked]
        candidates[algorithm] = (models[selected_method], result)
    chosen = min(candidates, key=lambda name: (candidates[name][1]["brier_score"], candidates[name][1]["expected_calibration_error"], -candidates[name][1]["simple_ranker_realized_net_minor"]))
    model = candidates[chosen][0]
    output_dir.mkdir(parents=True, exist_ok=True)
    artifact_path = output_dir / f"{MODEL_VERSION.replace('-', '_')}.joblib"
    bundle = {"model": model, "model_version": MODEL_VERSION, "feature_version": FEATURE_VERSION, "feature_names": FEATURE_NAMES, "algorithm": chosen}
    joblib.dump(bundle, artifact_path)
    selected_calibration = candidates[chosen][1]["calibration_method"]
    metadata = {
        "model_version": MODEL_VERSION, "feature_version": FEATURE_VERSION, "algorithm": chosen,
        "candidate_metrics": {name: result for name, (_, result) in candidates.items()},
        "calibration": {"method": selected_calibration, "methods_compared": ["raw", "sigmoid", "isotonic"], "dataset": "first half of validation split", "selection_dataset": "second half of validation split"},
        "training_dataset": {"path": str(train_path), "sha256": file_sha256(train_path), "rows": len(train_rows)},
        "validation_dataset": {"path": str(validation_path), "sha256": file_sha256(validation_path), "rows": len(validation_rows)},
        "held_out_test_used": False, "training_seed": seed,
        "training_timestamp": datetime.now(timezone.utc).isoformat(), "artifact": str(artifact_path),
    }
    (output_dir / f"{MODEL_VERSION.replace('-', '_')}_metadata.json").write_text(json.dumps(metadata, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return metadata


def main() -> None:
    parser = argparse.ArgumentParser(description="Train and select the action-conditioned recovery model")
    parser.add_argument("--train", type=Path, required=True)
    parser.add_argument("--validation", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, default=Path("models"))
    parser.add_argument("--seed", type=int, default=42)
    args = parser.parse_args()
    print(json.dumps(train(args.train, args.validation, args.output_dir, args.seed), indent=2, sort_keys=True))


if __name__ == "__main__": main()
