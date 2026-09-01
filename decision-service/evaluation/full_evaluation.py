from __future__ import annotations

import argparse
import json
import math
from pathlib import Path
from statistics import mean, stdev
from typing import Any

from evaluation.baselines import run as run_baselines
from evaluation.environment import file_sha256, load_jsonl
from evaluation.nba import run as run_nba
from simulation.generator import SimulationConfig, generate_dataset

DEFAULT_SEEDS = (101, 202, 303, 404, 505)
HIDDEN_FIELDS = {"liquidity_pattern", "retry_responsiveness", "payment_link_responsiveness",
                 "reminder_responsiveness", "contact_sensitivity", "promise_reliability",
                 "natural_recovery_probability", "payment_method_preference", "churn_intent"}


def allocate_same_budget(candidates: list[dict[str, Any]], greedy: bool) -> dict[str, Any]:
    """Compare allocation policies under one frozen, explicitly reported budget."""
    total_cost = sum(int(item["expected_cost_minor"]) for item in candidates)
    budget = {
        "spend_minor": total_cost // 2,
        "contacts": sum(bool(item["is_contact"]) for item in candidates) // 2,
        "retries": sum(bool(item["is_retry"]) for item in candidates) // 2,
    }
    if greedy:
        ordered = sorted(candidates, key=lambda item: (
            -(int(item["expected_nerv_minor"]) / max(1, int(item["expected_cost_minor"]))),
            -int(item["expected_nerv_minor"]), item["case_id"], item["sequence"],
        ))
    else:
        ordered = sorted(candidates, key=lambda item: (item["sequence"], item["case_id"]))
    selected, spend, contacts, retries = [], 0, 0, 0
    for item in ordered:
        next_spend = spend + int(item["expected_cost_minor"])
        next_contacts = contacts + int(bool(item["is_contact"]))
        next_retries = retries + int(bool(item["is_retry"]))
        if next_spend > budget["spend_minor"] or next_contacts > budget["contacts"] or next_retries > budget["retries"]:
            continue
        selected.append(item); spend, contacts, retries = next_spend, next_contacts, next_retries
    return {
        "policy": "greedy_nerv_efficiency" if greedy else "first_come_first_served",
        "budget": budget,
        "used": {"spend_minor": spend, "contacts": contacts, "retries": retries},
        "selected_cases": len(selected),
        "expected_nerv_minor": sum(int(item["expected_nerv_minor"]) for item in selected),
        "expected_incremental_value_minor": sum(int(item["expected_incremental_value_minor"]) for item in selected),
    }


def summarize(values: list[float]) -> dict[str, float]:
    average = mean(values); deviation = stdev(values) if len(values) > 1 else 0.0
    margin = 1.96 * deviation / math.sqrt(len(values)) if values else 0.0
    return {"mean": average, "stddev": deviation, "ci95_lower": average - margin, "ci95_upper": average + margin,
            "minimum": min(values), "maximum": max(values)}


def run(output_dir: Path, dataset_size: int = 5000, seeds: tuple[int, ...] = DEFAULT_SEEDS,
        outcome_artifact: Path = Path("models/candidates/phase21/outcome_v1.joblib"),
        natural_artifact: Path = Path("models/candidates/phase21/natural_recovery_v1.joblib")) -> dict[str, Any]:
    output_dir.mkdir(parents=True, exist_ok=True); all_results=[]; hashes={}; integrity=[]; budget_rows=[]
    generated_total = 0; heldout_total = 0
    for seed in seeds:
        seed_dir=output_dir/f"seed_{seed}";data_dir=seed_dir/"data";results_dir=seed_dir/"results"
        report=generate_dataset(SimulationConfig(seed=seed,dataset_size=dataset_size),data_dir)
        test_path=data_dir/"test.jsonl";train_path=data_dir/"train.jsonl";hashes[str(seed)]=file_sha256(test_path)
        rows=load_jsonl(test_path,"test")
        leaked=sorted({key for row in rows for key in row["observable"] if key in HIDDEN_FIELDS})
        if leaked: raise AssertionError(f"hidden features entered optimizer input: {leaked}")
        comparison=run_baselines(test_path,train_path,seed,results_dir)
        nba=run_nba(test_path,outcome_artifact,natural_artifact,results_dir/"baseline_comparison.json",results_dir/"nba.json",seed)
        per_seed=list(comparison["baselines"])+[nba];all_results.extend({"seed":seed,**value} for value in per_seed)
        generated_total += int(report["dataset_size"]); heldout_total += int(report["split_sizes"]["test"])
        fcfs = allocate_same_budget(nba["portfolio_candidates"], False); greedy = allocate_same_budget(nba["portfolio_candidates"], True)
        budget_rows.append({"seed": seed, "fcfs": fcfs, "greedy": greedy,
                            "expected_nerv_gain_minor": greedy["expected_nerv_minor"]-fcfs["expected_nerv_minor"]})
        integrity.append({"seed":seed,"configured_size":dataset_size,"actual_generated_size":report["dataset_size"],"actual_heldout_size":report["split_sizes"]["test"],"test_sha256":hashes[str(seed)],"hidden_feature_leak":False})
    chart_rows=[];grouped={}
    for result in all_results:
        strategy=result["baseline"]
        for vertical,metric in result["metrics"].items():
            net=metric["total_revenue_recovered_minor"]-metric["intervention_cost_minor"]
            row={"seed":result["seed"],"strategy":strategy,"vertical":vertical,"recovered_minor":metric["total_revenue_recovered_minor"],"net_recovered_minor":net,"recovery_rate":metric["recovery_rate"],"intervention_cost_minor":metric["intervention_cost_minor"],"contacts":metric["customer_contacts"],"attempts":metric["recovery_attempts"]}
            chart_rows.append(row);grouped.setdefault((strategy,vertical),[]).append(row)
    statistics={}
    for (strategy,vertical),rows in sorted(grouped.items()):
        statistics.setdefault(strategy,{})[vertical]={metric:summarize([float(row[metric]) for row in rows]) for metric in ("recovered_minor","net_recovered_minor","recovery_rate","intervention_cost_minor","contacts","attempts")}
    fcfs_nerv = sum(row["fcfs"]["expected_nerv_minor"] for row in budget_rows); greedy_nerv = sum(row["greedy"]["expected_nerv_minor"] for row in budget_rows)
    budget_comparison = {"comparison_version":"same-budget-allocation-v1","per_seed":budget_rows,
                         "aggregate":{"fcfs_expected_nerv_minor":fcfs_nerv,"greedy_expected_nerv_minor":greedy_nerv,
                                      "greedy_gain_minor":greedy_nerv-fcfs_nerv,
                                      "greedy_gain_percent":round((greedy_nerv-fcfs_nerv)*100/max(1,abs(fcfs_nerv)),6)}}
    result={"evaluation_version":"phase24-evaluation-v2","strategy_name":"full_nba_agent_v1","seeds":list(seeds),"dataset_size_per_seed":dataset_size,
            "case_counts":{"generated":generated_total,"heldout_evaluated_per_strategy":heldout_total,"heldout_per_seed":[item["actual_heldout_size"] for item in integrity]},
            "artifacts":{"outcome":{"path":str(outcome_artifact),"sha256":file_sha256(outcome_artifact)},"natural":{"path":str(natural_artifact),"sha256":file_sha256(natural_artifact)}},
            "integrity":{"checks":integrity,"distinct_test_hashes":len(set(hashes.values())),"all_checks_passed":all(item["actual_generated_size"]==dataset_size and not item["hidden_feature_leak"] for item in integrity)},
            "budget_comparison":budget_comparison,"statistics":statistics,"chart_rows":chart_rows,"limitations":["Synthetic potential outcomes support policy comparison, not production causal claims.","Intervals quantify variation across the configured deterministic simulation seeds; they are not production-population confidence claims."]}
    (output_dir/"summary.json").write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    (output_dir/"budget_comparison.json").write_text(json.dumps(budget_comparison,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    (output_dir/"chart_data.jsonl").write_text("".join(json.dumps(row,sort_keys=True)+"\n" for row in chart_rows),encoding="utf-8")
    return result


def main() -> None:
    parser=argparse.ArgumentParser(description="Run the complete five-seed recovery evaluation")
    parser.add_argument("--output-dir",type=Path,default=Path("evaluation/results/phase24"));parser.add_argument("--dataset-size",type=int,default=5000)
    parser.add_argument("--seeds",type=int,nargs="+",default=list(DEFAULT_SEEDS));parser.add_argument("--outcome-artifact",type=Path,default=Path("models/candidates/phase21/outcome_v1.joblib"));parser.add_argument("--natural-artifact",type=Path,default=Path("models/candidates/phase21/natural_recovery_v1.joblib"))
    args=parser.parse_args();result=run(args.output_dir,args.dataset_size,tuple(args.seeds),args.outcome_artifact,args.natural_artifact);print(json.dumps({"output":str(args.output_dir/"summary.json"),"integrity":result["integrity"]},indent=2))
if __name__=="__main__":main()
