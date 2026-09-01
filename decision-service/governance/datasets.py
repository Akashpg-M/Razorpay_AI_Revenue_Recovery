from __future__ import annotations

import hashlib
import json
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable


@dataclass(frozen=True)
class DatasetManifest:
    version: str
    row_count: int
    feature_version: str
    label_version: str
    data_sha256: str
    split_counts: dict[str, int]
    exclusions: dict[str, int]
    created_at: str


def canonical_json(value: Any) -> str:
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def build_versioned_dataset(records: Iterable[dict[str, Any]], output_dir: Path, version: str,
                            feature_version: str, label_version: str) -> DatasetManifest:
    """Create immutable chronological train/validation/test files from feedback only."""
    output_dir = output_dir / version
    if output_dir.exists():
        raise FileExistsError(f"dataset version already exists: {version}")
    eligible, exclusions = [], {}
    for record in records:
        reasons = list(record.get("exclusion_reasons", []))
        if not record.get("training_eligible", False):
            reasons = reasons or ["NOT_TRAINING_ELIGIBLE"]
        if reasons:
            for reason in reasons:
                exclusions[reason] = exclusions.get(reason, 0) + 1
            continue
        if record.get("environment") != "production":
            exclusions["NON_PRODUCTION"] = exclusions.get("NON_PRODUCTION", 0) + 1
            continue
        eligible.append(record)
    eligible.sort(key=lambda row: (row["created_at"], row.get("id", "")))
    output_dir.mkdir(parents=True)
    boundaries = (int(len(eligible) * .7), int(len(eligible) * .85))
    groups = {"train": eligible[:boundaries[0]], "validation": eligible[boundaries[0]:boundaries[1]], "test": eligible[boundaries[1]:]}
    digest = hashlib.sha256()
    for split, rows in groups.items():
        with (output_dir / f"{split}.jsonl").open("x", encoding="utf-8", newline="\n") as handle:
            for row in rows:
                encoded = canonical_json({**row, "split": split})
                digest.update((encoded + "\n").encode())
                handle.write(encoded + "\n")
    manifest = DatasetManifest(version, len(eligible), feature_version, label_version, digest.hexdigest(),
                               {key: len(value) for key, value in groups.items()}, dict(sorted(exclusions.items())),
                               datetime.now(timezone.utc).isoformat())
    (output_dir / "manifest.json").write_text(json.dumps(manifest.__dict__, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return manifest
