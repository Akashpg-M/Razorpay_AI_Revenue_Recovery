from __future__ import annotations

import argparse
import copy
import csv
import json
from collections import Counter
from pathlib import Path
from statistics import mean
from typing import Any

import joblib

from evaluation.environment import CONTACT_ACTIONS, file_sha256, load_jsonl, resolve
from evaluation.nba import ACTION_COSTS, FINAL_STRATEGY_NAME, OPTIMIZER_VERSION, eligible_actions, rank_candidates
from evaluation.contracts import Decision
from features.pipeline import FEATURE_VERSION, from_simulator
from simulation.generator import CHECKOUT_ACTIONS, SUBSCRIPTION_ACTIONS, SimulationConfig, generate_dataset

DEFAULT_SEEDS = (101, 202, 303, 404, 505)
CONFIGURATIONS = (
    FINAL_STRATEGY_NAME,
    "without_customer_context", "without_merchant_context", "without_natural_recovery_model",
    "without_fatigue_cost", "without_non_retry_actions", "without_ptp",
    "without_economic_gate", "without_policy_aware_optimization", "without_calibration",
)


def _neutralize(observable: dict[str, Any], configuration: str) -> dict[str, Any]:
    value = copy.deepcopy(observable)
    if configuration == "without_customer_context":
        value.update({"customer_segment": "UNKNOWN", "subscription_tenure_days": 0, "failure_count_90d": 0,
                      "past_recovery_actions": [], "contact_history": {"last_7d": 0, "last_contact_hours_ago": 0}})
        value["payment_history"] = {"failed_count": 0, "successful_count": 0, "success_rate": .5}
    elif configuration == "without_merchant_context":
        value["merchant_type"] = "UNKNOWN"
    return value


def _uncalibrated(model: Any) -> Any:
    candidate = getattr(model, "estimator", model)
    return getattr(candidate, "estimator", candidate)


def _allowed(observable: dict[str, Any], configuration: str) -> tuple[str, ...]:
    if configuration == "without_policy_aware_optimization":
        actions = SUBSCRIPTION_ACTIONS if observable["leak_type"] == "FAILED_SUBSCRIPTION" else CHECKOUT_ACTIONS
        return tuple(action for action in actions if action != "WAIT")
    actions = eligible_actions(observable)
    if configuration == "without_non_retry_actions":
        actions = tuple(action for action in actions if action.startswith("RETRY"))
    if configuration == "without_ptp":
        actions = tuple(action for action in actions if action != "WAIT_FOR_PROMISE_TO_PAY")
    return actions


def decide_all(rows: list[dict[str, Any]], outcome_bundle: dict[str, Any], natural_bundle: dict[str, Any], configuration: str) -> dict[str, Decision]:
    transformed = [_neutralize(row["observable"], configuration) for row in rows]
    use_raw = configuration == "without_calibration"
    outcome_model = _uncalibrated(outcome_bundle["model"]) if use_raw else outcome_bundle["model"]
    natural_model = _uncalibrated(natural_bundle["model"]) if use_raw else natural_bundle["model"]
    natural = natural_model.predict_proba([from_simulator(item, "WAIT") for item in transformed])[:, 1]
    coordinates, features = [], []
    for index, observable in enumerate(transformed):
        for action in _allowed(observable, configuration):
            coordinates.append((index, action)); features.append(from_simulator(observable, action))
    probabilities: list[dict[str, float]] = [{} for _ in rows]
    if features:
        for (index, action), probability in zip(coordinates, outcome_model.predict_proba(features)[:, 1]):
            probabilities[index][action] = float(probability)
    decisions: dict[str, Decision] = {}
    for index, row in enumerate(rows):
        natural_probability = .25 if configuration == "without_natural_recovery_model" else float(natural[index])
        ranked = rank_candidates(transformed[index], probabilities[index], natural_probability)
        if configuration == "without_fatigue_cost":
            for item in ranked:
                item["nerv_minor"] += item["fatigue_penalty_minor"]
                item["fatigue_penalty_minor"] = 0
            ranked.sort(key=lambda item: (-item["nerv_minor"], item["action"]))
        selected = ranked[0]
        if configuration != "without_economic_gate" and selected["action"] != "WAIT" and selected["nerv_minor"] < 0:
            selected = next(item for item in ranked if item["action"] == "WAIT")
        decisions[row["observable"]["case_id"]] = Decision(selected["action"], reason=configuration)
    return decisions


def measure(rows: list[dict[str, Any]], decisions: dict[str, Decision], seed: int) -> tuple[dict[str, Any], dict[str, dict[str, float]]]:
    totals = Counter(); per_case = {}
    for row in rows:
        case_id = row["observable"]["case_id"]; decision = decisions[case_id]; outcome = resolve(row, decision, seed)
        truth = row["_ground_truth"]["potential_outcomes"].get(decision.canonical_action, {})
        fatigue = int(truth.get("fatigue_cost_minor", 0)); amount = int(outcome.recovered_amount_minor)
        values = {"recovered_cases": int(outcome.recovered), "recovered_minor": amount,
                  "net_recovered_minor": amount-int(outcome.intervention_cost_minor), "intervention_cost_minor": int(outcome.intervention_cost_minor),
                  "contacts": int(outcome.is_contact), "attempts": int(outcome.is_attempt), "latency_hours": int(outcome.latency_hours),
                  "fatigue_cost_minor": fatigue, "waits": int(decision.action == "WAIT")}
        totals.update(values); per_case[case_id] = values
    result = dict(totals); result["cases"] = len(rows); result["recovery_rate"] = round(totals["recovered_cases"]/max(1,len(rows)), 8)
    result["average_latency_hours"] = round(totals["latency_hours"]/max(1,len(rows)), 6)
    return result, per_case


def run(output_dir: Path, dataset_size: int, seeds: tuple[int, ...], outcome_artifact: Path, natural_artifact: Path) -> dict[str, Any]:
    output_dir.mkdir(parents=True, exist_ok=True); per_seed_dir = output_dir/"per_seed"; per_seed_dir.mkdir(exist_ok=True)
    outcome_bundle, natural_bundle = joblib.load(outcome_artifact), joblib.load(natural_artifact)
    rows_summary, paired_rows, test_hashes, case_ids_by_seed = [], [], {}, {}
    for seed in seeds:
        data_dir = output_dir/"datasets"/f"seed_{seed}"
        report = generate_dataset(SimulationConfig(seed=seed,dataset_size=dataset_size), data_dir)
        test_path=data_dir/"test.jsonl"; rows=load_jsonl(test_path,"test"); test_hashes[str(seed)]=file_sha256(test_path)
        case_ids_by_seed[str(seed)] = [row["observable"]["case_id"] for row in rows]
        measured, cases = {}, {}
        for configuration in CONFIGURATIONS:
            decisions=decide_all(rows,outcome_bundle,natural_bundle,configuration); measured[configuration],cases[configuration]=measure(rows,decisions,seed)
            rows_summary.append({"seed":seed,"configuration":configuration,**measured[configuration]})
        for configuration in CONFIGURATIONS[1:]:
            for case_id in case_ids_by_seed[str(seed)]:
                paired_rows.append({"seed":seed,"case_id":case_id,"ablation":configuration,
                                    **{f"delta_{key}":cases[FINAL_STRATEGY_NAME][case_id][key]-cases[configuration][case_id][key]
                                       for key in cases[FINAL_STRATEGY_NAME][case_id]}})
        (per_seed_dir/f"seed_{seed}.json").write_text(json.dumps({"seed":seed,"test_sha256":test_hashes[str(seed)],"metrics":measured},indent=2,sort_keys=True)+"\n",encoding="utf-8")
    aggregate=[]
    for configuration in CONFIGURATIONS:
        selected=[row for row in rows_summary if row["configuration"]==configuration]
        aggregate.append({"configuration":configuration,**{key:round(mean(float(row[key]) for row in selected),6) for key in selected[0] if key not in {"seed","configuration"}}})
    full=next(item for item in aggregate if item["configuration"]==FINAL_STRATEGY_NAME)
    for item in aggregate:
        item["lost_net_recovered_minor_vs_full"] = round(full["net_recovered_minor"]-item["net_recovered_minor"],6)
    aggregate.sort(key=lambda item:-item["lost_net_recovered_minor_vs_full"])
    manifest={"evaluation_version":"phase25-paired-ablation-v1","configurations":list(CONFIGURATIONS),"seeds":list(seeds),
              "generated_cases":dataset_size*len(seeds),"heldout_cases":sum(len(ids) for ids in case_ids_by_seed.values()),
              "paired_case_identity_verified":all(len(ids)==len(set(ids)) for ids in case_ids_by_seed.values()),"test_sha256":test_hashes,
              "versions":{"feature":FEATURE_VERSION,"optimizer":OPTIMIZER_VERSION,"merchant_profile":"evaluation-default-profile-v1",
                          "outcome_model":outcome_bundle["model_version"],"natural_model":natural_bundle["model_version"]},
              "artifacts":{"outcome_sha256":file_sha256(outcome_artifact),"natural_sha256":file_sha256(natural_artifact)},
              "ablation_contract":"Each configuration changes only the named variable family; all share exact case IDs and potential outcomes."}
    summary={"manifest":manifest,"ranked_ablations":aggregate}
    (output_dir/"ablation_manifest.json").write_text(json.dumps(manifest,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    (output_dir/"ablation_summary.json").write_text(json.dumps(summary,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    (output_dir/"ablation_chart_data.json").write_text(json.dumps(aggregate,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    for name, records in (("ablation_summary.csv",rows_summary),("paired_deltas.csv",paired_rows)):
        with (output_dir/name).open("w",newline="",encoding="utf-8") as handle:
            writer=csv.DictWriter(handle,fieldnames=list(records[0])); writer.writeheader(); writer.writerows(records)
    return summary


def main() -> None:
    parser=argparse.ArgumentParser(); parser.add_argument("--output-dir",type=Path,default=Path("evaluation/results/phase25")); parser.add_argument("--dataset-size",type=int,default=5000)
    parser.add_argument("--seeds",type=int,nargs="+",default=list(DEFAULT_SEEDS)); parser.add_argument("--outcome-artifact",type=Path,default=Path("models/candidates/phase21/outcome_v1.joblib")); parser.add_argument("--natural-artifact",type=Path,default=Path("models/candidates/phase21/natural_recovery_v1.joblib"))
    args=parser.parse_args(); print(json.dumps(run(args.output_dir,args.dataset_size,tuple(args.seeds),args.outcome_artifact,args.natural_artifact)["manifest"],indent=2))

if __name__ == "__main__": main()
