# ML system

Two separately versioned predictors estimate action-conditional recovery and natural recovery from observable features only. `_ground_truth` simulator fields are rejected from feature paths. Calibration is validation-only and the frozen test split is used once for reporting. Go calculates incremental uplift and NERV, then policy can still deny, stop, or escalate.

Artifacts include model metadata, feature version, seed, hashes, and evaluation outputs under `decision-service/evaluation/results`. The current models are trained on synthetic data and are not evidence of production generalization.
