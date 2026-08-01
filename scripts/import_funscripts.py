#!/usr/bin/env python3
"""Generate experimental MagicHandy review patterns from adapter funscripts."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from collections import defaultdict
from dataclasses import dataclass
from pathlib import Path

MAX_SEGMENT_MS = 10_000
MIN_SEGMENT_MS = 3_000
MIN_MOTION_SPAN = 5.0
RESAMPLE_POINTS = 24
SIMILARITY_TOLERANCE = 6.0
TARGET_CURATED_DEFAULT = 171
UPSTREAM_BUILTIN_COUNT = 29
CATALOG_TARGET_TOTAL = 200

DEFAULT_SOURCE = (
    Path(__file__).resolve().parents[2]
    / "LSO---Local_Stroke_Orchestrator"
    / "jsonmodes"
    / "Myadapter"
)
DEFAULT_OUTPUT = (
    Path(__file__).resolve().parents[1]
    / "internal"
    / "motion"
    / "builtinpatterns"
    / "curated"
)
CATALOG_NAME = "_catalog.json"

POSE_LABELS = {
    "blowjob": "Blowjob",
    "handjob": "Handjob",
    "riding": "Cowgirl",
    "cowgirl": "Cowgirl",
    "milking": "Milking",
    "asmr": "ASMR",
}

ZONE_LABELS = {
    "tip": "Tip",
    "middle": "Mid",
    "shaft": "Shaft",
    "full": "Full",
    "deep": "Deep",
    "finish": "Finish",
    "edge": "Edge",
}

SPEED_LABELS = {
    "slow": "Slow",
    "medium": "Medium",
    "fast": "Fast",
    "very_fast": "Very Fast",
}

STYLE_LABELS = {
    "finisher": "Finisher",
    "edging": "Edging",
    "base": "Base",
    "intense": "Intense",
    "slow": "Slow",
    "mlking": "Milking",
    "tip": "Tip",
    "fast": "Fast",
    "flow": "Flow",
}

STEM_ZONE_HINTS = (
    ("bjtip", "tip"),
    ("handjob-tip", "tip"),
    ("-tip", "tip"),
    ("mlking", "shaft"),
    ("milking", "shaft"),
    ("finisher", "finish"),
    ("edging", "edge"),
    ("full-intense", "full"),
    ("full-slow", "full"),
    ("blowjobfull", "full"),
    ("cowgirl", "deep"),
    ("intense", "full"),
)

STEM_STYLE_HINTS = (
    "finisher",
    "edging",
    "base",
    "intense",
    "slow",
    "mlking",
    "tip",
    "fast",
)

ZONE_MOTION_TAGS = {
    "tip": "tip",
    "middle": "centered",
    "shaft": "paired",
    "full": "full",
    "deep": "deep",
    "finish": "accent",
    "edge": "pauses",
}

SPEED_MOTION_TAGS = {
    "slow": "gentle",
    "medium": "steady",
    "fast": "rhythmic",
    "very_fast": "fast",
}

STYLE_MOTION_TAGS = {
    "finisher": "tempo-change",
    "edging": "uneven",
    "base": "balanced",
    "intense": "build",
    "slow": "breathing",
    "mlking": "contrast",
    "tip": "short",
    "fast": "accelerating",
    "flow": "varied",
}

POSE_MOTION_TAGS = {
    "blowjob": "upper-return",
    "cowgirl": "rhythmic",
    "handjob": None,
    "milking": "paired",
    "asmr": "gentle",
}


def build_tags(candidate: SegmentCandidate) -> list[str]:
    tags: list[str] = []
    for tag in (
        ZONE_MOTION_TAGS.get(candidate.zone),
        SPEED_MOTION_TAGS.get(candidate.speed),
        STYLE_MOTION_TAGS.get(candidate.style),
        POSE_MOTION_TAGS.get(candidate.pose),
    ):
        if tag and tag not in tags:
            tags.append(tag)
    return tags[:4]
@dataclass
class SegmentCandidate:
    source_stem: str
    segment_index: int
    segment_count: int
    pose: str
    zone: str
    style: str
    speed: str
    cycle_ms: int
    points: list[dict]
    signature: tuple[float, ...]
    variant: str
    name_suffix: str = ""


def load_manifest(source_dir: Path) -> dict[str, dict]:
    manifest_path = source_dir / "_manifest.json"
    if not manifest_path.exists():
        return {}
    entries = json.loads(manifest_path.read_text(encoding="utf-8"))
    return {entry["file"]: entry for entry in entries}


def slugify(value: str) -> str:
    value = value.strip().lower()
    value = re.sub(r"[^a-z0-9]+", "-", value)
    return value.strip("-") or "pattern"


def infer_pose(meta: dict, metadata: dict, stem: str) -> str:
    stem_lower = stem.lower()
    if stem_lower.startswith("handjob") or stem_lower.startswith("hj"):
        return "handjob"
    if stem_lower.startswith("bj") or "blowjob" in stem_lower:
        return "blowjob"
    if "cowgirl" in stem_lower or stem_lower.startswith("riding"):
        return "cowgirl"
    if "mlking" in stem_lower or "milking" in stem_lower:
        return "milking"
    if "asmr" in stem_lower:
        return "asmr"

    pose = str(meta.get("pose") or metadata.get("pose") or "").lower()
    if pose in {"riding", "cowgirl"}:
        return "cowgirl"
    if pose == "milking":
        return "milking"
    if pose in POSE_LABELS:
        return pose
    return pose or "handjob"


def infer_zone(meta: dict, metadata: dict, stem: str) -> str:
    stem_lower = stem.lower()
    for hint, zone in STEM_ZONE_HINTS:
        if hint in stem_lower:
            return zone

    stroke_length = str(metadata.get("stroke_length") or "").lower()
    if stroke_length in {"tip", "short"}:
        return "tip"
    if stroke_length in {"medium", "mid", "middle"}:
        return "middle"
    if stroke_length in {"deep", "bottom"}:
        return "deep"
    if stroke_length == "full":
        return "full"

    zone = str(meta.get("zone") or metadata.get("zone") or "full").lower()
    if zone == "bottom":
        return "deep"
    if zone == "middle":
        return "middle"
    return zone if zone in ZONE_LABELS else "full"


def infer_style(stem: str, zone: str = "") -> str:
    stem_lower = stem.lower()
    for hint in STEM_STYLE_HINTS:
        if hint not in stem_lower:
            continue
        if hint == "tip" and zone == "tip":
            continue
        return hint
    return "flow"


def infer_speed(meta: dict, metadata: dict) -> str:
    speed = str(meta.get("speed") or metadata.get("speed") or "medium").lower()
    return speed if speed in SPEED_LABELS else "medium"


def infer_variant(stem: str) -> str:
    match = re.search(r"(?:^|-)v(\d+)$", stem.lower())
    if match:
        return f"v{match.group(1)}"
    return ""


def stem_version(stem: str) -> int:
    match = re.search(r"(?:^|-)v(\d+)$", stem.lower())
    if match:
        return int(match.group(1))
    return 0


def parse_actions(raw: dict) -> list[dict]:
    inverted = bool(raw.get("inverted", False))
    actions = []
    for action in raw.get("actions", []):
        at = int(action["at"])
        pos = float(action["pos"])
        if inverted:
            pos = 100.0 - pos
        actions.append({"at": at, "pos": pos})
    actions.sort(key=lambda item: item["at"])
    deduped: list[dict] = []
    for action in actions:
        if deduped and deduped[-1]["at"] == action["at"]:
            deduped[-1] = action
        else:
            deduped.append(action)
    return deduped


def split_actions(actions: list[dict], max_ms: int = MAX_SEGMENT_MS) -> list[list[dict]]:
    if len(actions) < 2:
        return []
    total = actions[-1]["at"]
    if total <= max_ms:
        return [actions]

    chunks: list[list[dict]] = []
    chunk_start = 0
    while chunk_start < total:
        chunk_end = min(chunk_start + max_ms, total)
        subset: list[dict] = []
        prev = None
        for action in actions:
            if action["at"] < chunk_start:
                prev = action
            elif action["at"] <= chunk_end:
                subset.append({"at": action["at"], "pos": action["pos"]})

        if prev and (not subset or subset[0]["at"] > chunk_start):
            subset.insert(0, {"at": chunk_start, "pos": prev["pos"]})
        if not subset and prev:
            subset = [{"at": chunk_start, "pos": prev["pos"]}]
        if subset and subset[-1]["at"] < chunk_end:
            subset.append({"at": chunk_end, "pos": subset[-1]["pos"]})

        retimed = [{"at": action["at"] - chunk_start, "pos": action["pos"]} for action in subset]
        if len(retimed) >= 2 and retimed[-1]["at"] > retimed[0]["at"]:
            chunks.append(retimed)
        if chunk_end >= total:
            break
        chunk_start = chunk_end
    return chunks


def to_curve_points(segment: list[dict]) -> list[dict]:
    minimum = min(action["pos"] for action in segment)
    maximum = max(action["pos"] for action in segment)
    span = maximum - minimum
    if span < MIN_MOTION_SPAN:
        raise ValueError("segment has no usable motion span")
    points = []
    for action in segment:
        normalized = (action["pos"] - minimum) * 100.0 / span
        points.append(
            {
                "time_ms": int(action["at"]),
                "position_percent": round(normalized, 6),
            }
        )
    return points


def resample_signature(points: list[dict], samples: int = RESAMPLE_POINTS) -> tuple[float, ...]:
    if len(points) < 2:
        return tuple()
    duration = points[-1]["time_ms"]
    if duration <= 0:
        return tuple()

    output: list[float] = []
    for index in range(samples):
        target = index * duration / (samples - 1)
        for cursor in range(1, len(points)):
            next_point = points[cursor]
            if next_point["time_ms"] >= target:
                previous = points[cursor - 1]
                start_ms = previous["time_ms"]
                end_ms = next_point["time_ms"]
                if end_ms == start_ms:
                    value = previous["position_percent"]
                else:
                    ratio = (target - start_ms) / (end_ms - start_ms)
                    value = previous["position_percent"] + (
                        (next_point["position_percent"] - previous["position_percent"]) * ratio
                    )
                output.append(round(value, 1))
                break
        else:
            output.append(round(points[-1]["position_percent"], 1))
    return tuple(output)


def signatures_similar(left: tuple[float, ...], right: tuple[float, ...], tolerance: float) -> bool:
    if not left or not right or len(left) != len(right):
        return False
    return max(abs(a - b) for a, b in zip(left, right)) <= tolerance


def bucket_key(candidate: SegmentCandidate) -> tuple[str, str, str, str]:
    return candidate.pose, candidate.zone, candidate.style, candidate.speed


def build_display_name(candidate: SegmentCandidate) -> str:
    pose = POSE_LABELS.get(candidate.pose, candidate.pose.title())
    zone = ZONE_LABELS.get(candidate.zone, candidate.zone.title())
    style = STYLE_LABELS.get(candidate.style, candidate.style.title())
    speed = SPEED_LABELS.get(candidate.speed, candidate.speed.replace("_", " ").title())
    duration = candidate.cycle_ms / 1000.0
    parts = [pose, zone, style, speed, f"{duration:.1f}s"]
    if candidate.variant:
        parts.insert(4, candidate.variant)
    if candidate.name_suffix:
        parts.insert(4, candidate.name_suffix)
    if candidate.segment_count > 1:
        parts.append(f"Part {candidate.segment_index}/{candidate.segment_count}")
    return " · ".join(parts)


def build_filename(candidate: SegmentCandidate) -> str:
    stem = slugify(candidate.source_stem)
    variant = slugify(candidate.variant) if candidate.variant else ""
    parts = [candidate.pose, candidate.zone, candidate.style, candidate.speed]
    if variant:
        parts.append(variant)
    parts.append(stem)
    parts.append(f"s{candidate.segment_index:02d}")
    return "-".join(slugify(part) for part in parts if part) + ".mhpattern.json"


def build_description(candidate: SegmentCandidate) -> str:
    return (
        f"{candidate.cycle_ms / 1000:.1f}s motion clip from {candidate.source_stem} · "
        f"pose={candidate.pose} zone={candidate.zone} style={candidate.style} speed={candidate.speed}"
    )


def build_pattern(candidate: SegmentCandidate) -> dict:
    return {
        "schema": "magichandy.pattern.v1",
        "name": build_display_name(candidate),
        "description": build_description(candidate),
        "kind": "routine",
        "cycle_ms": candidate.cycle_ms,
        "points": candidate.points,
        "tags": build_tags(candidate),
    }


def load_existing_signatures(output_dir: Path) -> list[tuple[float, ...]]:
    signatures: list[tuple[float, ...]] = []
    for path in sorted(output_dir.glob("*.mhpattern.json")):
        payload = json.loads(path.read_text(encoding="utf-8"))
        signatures.append(resample_signature(payload.get("points", [])))
    return [signature for signature in signatures if signature]


def collect_candidates(source_dir: Path) -> tuple[list[SegmentCandidate], list[str]]:
    manifest = load_manifest(source_dir)
    candidates: list[SegmentCandidate] = []
    issues: list[str] = []

    for path in sorted(source_dir.glob("*.funscript")):
        meta = manifest.get(path.name, {})
        raw = json.loads(path.read_text(encoding="utf-8"))
        metadata = raw.get("metadata", {})
        actions = parse_actions(raw)
        segments = split_actions(actions)
        if not segments:
            issues.append(f"{path.name}: no usable segments")
            continue

        pose = infer_pose(meta, metadata, path.stem)
        zone = infer_zone(meta, metadata, path.stem)
        style = infer_style(path.stem, zone)
        speed = infer_speed(meta, metadata)
        variant = infer_variant(path.stem)

        for index, segment in enumerate(segments, start=1):
            try:
                points = to_curve_points(segment)
            except ValueError as exc:
                issues.append(f"{path.name} part {index}: {exc}")
                continue

            cycle_ms = points[-1]["time_ms"]
            if cycle_ms < MIN_SEGMENT_MS:
                issues.append(f"{path.name} part {index}: segment shorter than {MIN_SEGMENT_MS}ms")
                continue

            signature = resample_signature(points)
            if not signature:
                issues.append(f"{path.name} part {index}: empty signature")
                continue

            candidates.append(
                SegmentCandidate(
                    source_stem=path.stem,
                    segment_index=index,
                    segment_count=len(segments),
                    pose=pose,
                    zone=zone,
                    style=style,
                    speed=speed,
                    cycle_ms=cycle_ms,
                    points=points,
                    signature=signature,
                    variant=variant,
                )
            )
    return candidates, issues


def dedupe_candidates(
    candidates: list[SegmentCandidate],
    existing_signatures: list[tuple[float, ...]],
    tolerance: float,
) -> tuple[list[SegmentCandidate], list[str]]:
    kept: list[SegmentCandidate] = []
    skipped: list[str] = []
    exact_seen: set[tuple[float, ...]] = set(existing_signatures)
    bucket_signatures: dict[tuple[str, str, str, str], list[tuple[float, ...]]] = {}

    for signature in existing_signatures:
        bucket_signatures.setdefault(("__existing__", "", "", ""), []).append(signature)

    for candidate in candidates:
        if candidate.signature in exact_seen:
            skipped.append(f"{candidate.source_stem} part {candidate.segment_index}: exact duplicate")
            continue

        bucket = bucket_key(candidate)
        similar_pool = list(existing_signatures)
        similar_pool.extend(bucket_signatures.get(bucket, []))
        if any(signatures_similar(candidate.signature, prior, tolerance) for prior in similar_pool):
            skipped.append(
                f"{candidate.source_stem} part {candidate.segment_index}: similar to existing {bucket} clip"
            )
            continue

        exact_seen.add(candidate.signature)
        bucket_signatures.setdefault(bucket, []).append(candidate.signature)
        kept.append(candidate)

    return kept, skipped


def stem_family(stem: str) -> str:
    return re.sub(r"-v\d+$", "", stem.lower())


def signature_distance(left: tuple[float, ...], right: tuple[float, ...]) -> float:
    if not left or not right or len(left) != len(right):
        return 0.0
    return max(abs(a - b) for a, b in zip(left, right))


def select_diverse_group(group: list[SegmentCandidate], quota: int) -> tuple[list[SegmentCandidate], list[str]]:
    if len(group) <= quota:
        return group, []

    ranked = sorted(
        group,
        key=lambda candidate: (
            -stem_version(candidate.source_stem),
            candidate.segment_index,
            candidate.source_stem,
        ),
    )
    selected = [ranked[0]]
    remaining = ranked[1:]
    selected_families = {stem_family(ranked[0].source_stem)}
    selected_segments = {ranked[0].segment_index}

    while len(selected) < quota and remaining:
        best_index = 0
        best_score = -1.0
        for index, candidate in enumerate(remaining):
            min_distance = min(signature_distance(candidate.signature, prior.signature) for prior in selected)
            family_bonus = 4.0 if stem_family(candidate.source_stem) not in selected_families else 0.0
            segment_bonus = 2.0 if candidate.segment_index not in selected_segments else 0.0
            speed_bonus = 1.0 if candidate.speed not in {prior.speed for prior in selected} else 0.0
            score = min_distance + family_bonus + segment_bonus + speed_bonus
            if score > best_score:
                best_score = score
                best_index = index
        chosen = remaining.pop(best_index)
        selected.append(chosen)
        selected_families.add(stem_family(chosen.source_stem))
        selected_segments.add(chosen.segment_index)

    skipped = [
        f"{candidate.source_stem} part {candidate.segment_index}: curated out for catalog budget"
        for candidate in group
        if candidate not in selected
    ]
    return selected, skipped


def curate_catalog(candidates: list[SegmentCandidate], target: int) -> tuple[list[SegmentCandidate], list[str]]:
    skipped: list[str] = []
    if len(candidates) <= target:
        return candidates, skipped

    buckets: dict[tuple[str, str, str], list[SegmentCandidate]] = defaultdict(list)
    for candidate in candidates:
        buckets[(candidate.pose, candidate.zone, candidate.style)].append(candidate)

    bucket_weights: dict[tuple[str, str, str], float] = {}
    for key, group in buckets.items():
        pose, zone, _style = key
        weight = float(len(group))
        if pose == "blowjob":
            weight *= 1.3
        elif pose == "cowgirl":
            weight *= 2.2
        elif pose == "handjob" and zone == "full":
            weight *= 0.42
        if zone in {"tip", "finish", "edge", "deep", "shaft", "middle"}:
            weight *= 1.2
        bucket_weights[key] = weight

    total_weight = sum(bucket_weights.values())
    quotas: dict[tuple[str, str, str], int] = {}
    for key, weight in bucket_weights.items():
        minimum = 2 if len(buckets[key]) >= 2 else 1
        quotas[key] = max(minimum, round(target * weight / total_weight))
        quotas[key] = min(quotas[key], len(buckets[key]))

    allocated = sum(quotas.values())
    while allocated > target:
        reducible = [key for key, quota in quotas.items() if quota > 1]
        if not reducible:
            break
        key = max(reducible, key=lambda item: quotas[item])
        quotas[key] -= 1
        allocated -= 1

    while allocated < target:
        expandable = [key for key, quota in quotas.items() if quota < len(buckets[key])]
        if not expandable:
            break
        key = max(expandable, key=lambda item: bucket_weights[item])
        quotas[key] += 1
        allocated += 1

    selected: list[SegmentCandidate] = []
    for key, quota in quotas.items():
        picked, group_skipped = select_diverse_group(buckets[key], quota)
        selected.extend(picked)
        skipped.extend(group_skipped)

    if len(selected) > target:
        selected.sort(
            key=lambda candidate: (
                bucket_weights.get((candidate.pose, candidate.zone, candidate.style), 0.0),
                signature_distance(
                    candidate.signature,
                    selected[0].signature if selected else candidate.signature,
                ),
            ),
            reverse=True,
        )
        for candidate in selected[target:]:
            skipped.append(f"{candidate.source_stem} part {candidate.segment_index}: trimmed to catalog target")
        selected = selected[:target]

    selected.sort(
        key=lambda candidate: (
            candidate.pose,
            candidate.zone,
            candidate.style,
            candidate.speed,
            candidate.source_stem,
            candidate.segment_index,
        )
    )
    return selected, skipped


def ensure_unique_names(candidates: list[SegmentCandidate]) -> None:
    seen: dict[str, int] = {}
    for candidate in candidates:
        base_name = build_display_name(candidate)
        count = seen.get(base_name, 0) + 1
        seen[base_name] = count
        if count > 1 and not candidate.name_suffix:
            candidate.name_suffix = slugify(candidate.source_stem)


def write_catalog(output_dir: Path, candidates: list[SegmentCandidate], skipped: list[str], issues: list[str]) -> None:
    catalog = {
        "schema": "magichandy.generated-pattern-catalog.v3",
        "status_policy": "runtime-budget-audit",
        "normal_speed_controls": True,
        "reason": "Generated clips remain available; problematic curves are experimental, unsafe source timing is resampled, and every curve passes normal catalog budgets without a bulk exemption.",
        "segment_ms_max": MAX_SEGMENT_MS,
        "target_total": CATALOG_TARGET_TOTAL,
        "upstream_builtin_count": UPSTREAM_BUILTIN_COUNT,
        "pattern_count": len(candidates),
        "patterns": [
            {
                "file": build_filename(candidate),
                "name": build_display_name(candidate),
                "source": candidate.source_stem,
                "pose": candidate.pose,
                "zone": candidate.zone,
                "style": candidate.style,
                "speed": candidate.speed,
            }
            for candidate in candidates
        ],
        "skipped": skipped,
        "issues": issues,
    }
    (output_dir / CATALOG_NAME).write_text(json.dumps(catalog, indent=2) + "\n", encoding="utf-8")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("source", nargs="?", default=str(DEFAULT_SOURCE))
    parser.add_argument("output", nargs="?", default=str(DEFAULT_OUTPUT))
    parser.add_argument("--clean", action="store_true", help="remove stale pattern files")
    parser.add_argument(
        "--tolerance",
        type=float,
        default=SIMILARITY_TOLERANCE,
        help="max resampled point delta to treat clips as duplicates",
    )
    parser.add_argument(
        "--target",
        type=int,
        default=TARGET_CURATED_DEFAULT,
        help="motion clip count to keep after curation",
    )
    parser.add_argument(
        "--total",
        type=int,
        default=CATALOG_TARGET_TOTAL,
        help="desired total library size including upstream builtins",
    )
    args = parser.parse_args()
    curated_target = args.target
    if args.total > UPSTREAM_BUILTIN_COUNT:
        curated_target = min(curated_target, args.total - UPSTREAM_BUILTIN_COUNT)

    source_dir = Path(args.source)
    output_dir = Path(args.output)
    if not source_dir.exists():
        print(f"source dir not found: {source_dir}", file=sys.stderr)
        return 1

    output_dir.mkdir(parents=True, exist_ok=True)
    existing_signatures = load_existing_signatures(output_dir) if not args.clean else []

    candidates, issues = collect_candidates(source_dir)
    candidates.sort(
        key=lambda candidate: (
            bucket_key(candidate),
            candidate.segment_index,
            -stem_version(candidate.source_stem),
            candidate.source_stem,
        )
    )
    kept, skipped = dedupe_candidates(candidates, existing_signatures, args.tolerance)
    kept, curated_skipped = curate_catalog(kept, curated_target)
    skipped.extend(curated_skipped)
    ensure_unique_names(kept)

    written_files: list[Path] = []
    for candidate in kept:
        filename = build_filename(candidate)
        payload = build_pattern(candidate)
        path = output_dir / filename
        path.write_text(json.dumps(payload, indent=2) + "\n", encoding="utf-8")
        written_files.append(path)

    removed = 0
    if args.clean:
        keep_names = {path.name for path in written_files}
        keep_names.add(CATALOG_NAME)
        for path in output_dir.glob("*.mhpattern.json"):
            if path.name not in keep_names:
                path.unlink()
                removed += 1

    write_catalog(output_dir, kept, skipped, issues)

    print(
        "done:"
        f" candidates={len(candidates)}"
        f" kept={len(kept)}"
        f" target={curated_target}"
        f" skipped={len(skipped)}"
        f" removed={removed}"
        f" output={output_dir}"
    )
    if skipped:
        print("skipped:", file=sys.stderr)
        for item in skipped[:15]:
            print(f"  - {item}", file=sys.stderr)
        if len(skipped) > 15:
            print(f"  - ... and {len(skipped) - 15} more", file=sys.stderr)
    if issues:
        print("issues:", file=sys.stderr)
        for item in issues[:15]:
            print(f"  - {item}", file=sys.stderr)
        if len(issues) > 15:
            print(f"  - ... and {len(issues) - 15} more", file=sys.stderr)
    return 0 if kept else 1


if __name__ == "__main__":
    raise SystemExit(main())
