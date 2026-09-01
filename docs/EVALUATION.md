# Evaluation

The deterministic simulator is evaluated against no-action, fixed retry, contextual retry, and explicit-rule baselines on identical held-out populations. Multi-seed paired comparisons, ablations, safety invariants, and reliability scenarios are stored under `decision-service/evaluation/results/phase25` through `phase27`.

Run `python -m evaluation.full_evaluation --dataset-size 5000 --seeds 101 202 303 404 505` and `go run ./cmd/evaluation ../decision-service/evaluation/results`. Report synthetic means and dispersion as simulator measurements, never production lift.
