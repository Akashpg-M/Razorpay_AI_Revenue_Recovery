from __future__ import annotations

from typing import Any
import numpy as np
from sklearn.isotonic import IsotonicRegression
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import brier_score_loss


class ProbabilityCalibrator:
    def __init__(self, method: str):
        if method not in {"raw", "sigmoid", "isotonic"}: raise ValueError("unknown calibration method")
        self.method, self.model = method, None

    def fit(self, validation_probability: list[float], validation_label: list[int]) -> "ProbabilityCalibrator":
        x, y = np.asarray(validation_probability), np.asarray(validation_label)
        if self.method == "sigmoid": self.model = LogisticRegression().fit(x.reshape(-1, 1), y)
        elif self.method == "isotonic": self.model = IsotonicRegression(out_of_bounds="clip").fit(x, y)
        return self

    def predict(self, probability: list[float]) -> np.ndarray:
        x = np.asarray(probability)
        if self.method == "raw": return np.clip(x, 0, 1)
        if self.method == "sigmoid": return self.model.predict_proba(x.reshape(-1, 1))[:, 1]
        return self.model.predict(x)


def calibration_report(probability: list[float], label: list[int], segments: list[str] | None = None,
                       bins: int = 10) -> dict[str, Any]:
    p, y = np.asarray(probability), np.asarray(label)
    edges = np.linspace(0, 1, bins + 1); rows=[]; ece=0.0
    for index in range(bins):
        mask=(p>=edges[index]) & ((p<=edges[index+1]) if index==bins-1 else (p<edges[index+1]))
        if not mask.any(): continue
        predicted=float(p[mask].mean()); actual=float(y[mask].mean()); weight=float(mask.mean());ece+=weight*abs(predicted-actual)
        rows.append({"lower":float(edges[index]),"upper":float(edges[index+1]),"count":int(mask.sum()),"predicted":predicted,"actual":actual})
    report={"brier_score":float(brier_score_loss(y,p)),"expected_calibration_error":ece,"bins":rows}
    if segments is not None:
        segment_array=np.asarray(segments);report["segments"]={}
        for segment in sorted(set(segments)):
            mask=segment_array==segment
            report["segments"][segment]={"count":int(mask.sum()),"brier_score":float(brier_score_loss(y[mask],p[mask]))}
    return report


def select_on_validation(validation_probability: list[float], validation_label: list[int]) -> tuple[ProbabilityCalibrator, dict[str, Any]]:
    candidates=[]
    for method in ("raw","sigmoid","isotonic"):
        fitted=ProbabilityCalibrator(method).fit(validation_probability,validation_label);calibrated=fitted.predict(validation_probability)
        candidates.append((calibration_report(calibrated.tolist(),validation_label),fitted))
    report,chosen=min(candidates,key=lambda item:(item[0]["brier_score"],item[0]["expected_calibration_error"],item[1].method))
    return chosen,{"selection_split":"validation","chosen_method":chosen.method,"validation":report}
