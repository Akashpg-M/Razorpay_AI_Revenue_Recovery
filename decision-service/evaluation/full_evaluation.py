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


def summarize(values: list[float]) -> dict[str, float]:
    average = mean(values); deviation = stdev(values) if len(values) > 1 else 0.0
    margin = 1.96 * deviation / math.sqrt(len(values)) if values else 0.0
    return {"mean": average, "stddev": deviation, "ci95_lower": average - margin, "ci95_upper": average + margin,
            "minimum": min(values), "maximum": max(values)}


def run(output_dir: Path, dataset_size: int = 5000, seeds: tuple[int, ...] = DEFAULT_SEEDS,
        outcome_artifact: Path = Path("models/candidates/phase21/outcome_v1.joblib"),
        natural_artifact: Path = Path("models/candidates/phase21/natural_recovery_v1.joblib")) -> dict[str, Any]:
    output_dir.mkdir(parents=True, exist_ok=True); all_results=[]; hashes={}; integrity=[]
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
        integrity.append({"seed":seed,"configured_size":dataset_size,"actual_size":report["dataset_size"],"test_sha256":hashes[str(seed)],"hidden_feature_leak":False})
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
    result={"evaluation_version":"phase24-evaluation-v1","seeds":list(seeds),"dataset_size_per_seed":dataset_size,
            "artifacts":{"outcome":{"path":str(outcome_artifact),"sha256":file_sha256(outcome_artifact)},"natural":{"path":str(natural_artifact),"sha256":file_sha256(natural_artifact)}},
            "integrity":{"checks":integrity,"distinct_test_hashes":len(set(hashes.values())),"all_checks_passed":all(item["actual_size"]==dataset_size and not item["hidden_feature_leak"] for item in integrity)},
            "statistics":statistics,"chart_rows":chart_rows,"limitations":["Synthetic potential outcomes support policy comparison, not production causal claims.","Confidence intervals summarize seed variation only."]}
    (output_dir/"summary.json").write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    (output_dir/"chart_data.jsonl").write_text("".join(json.dumps(row,sort_keys=True)+"\n" for row in chart_rows),encoding="utf-8")
    return result


def main() -> None:
    parser=argparse.ArgumentParser(description="Run the complete five-seed recovery evaluation")
    parser.add_argument("--output-dir",type=Path,default=Path("evaluation/results/phase24"));parser.add_argument("--dataset-size",type=int,default=5000)
    parser.add_argument("--seeds",type=int,nargs="+",default=list(DEFAULT_SEEDS));parser.add_argument("--outcome-artifact",type=Path,default=Path("models/candidates/phase21/outcome_v1.joblib"));parser.add_argument("--natural-artifact",type=Path,default=Path("models/candidates/phase21/natural_recovery_v1.joblib"))
    args=parser.parse_args();result=run(args.output_dir,args.dataset_size,tuple(args.seeds),args.outcome_artifact,args.natural_artifact);print(json.dumps({"output":str(args.output_dir/"summary.json"),"integrity":result["integrity"]},indent=2))
if __name__=="__main__":main()
