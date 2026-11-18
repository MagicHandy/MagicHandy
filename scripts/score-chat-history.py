"""Score a numeric chat replay against local annotations keyed by session and step."""

import argparse
from collections import Counter, defaultdict
import json
from pathlib import Path
import statistics


PACE = {"up", "down", "max", "hold", "set", "adjust"}
PROFILE_FIELDS = ("schema", "source_id", "labels_bound", "model", "provider", "capability", "context_size", "history_messages", "history_bytes",
                  "settings", "infer_stops", "seed_mode", "requested_sampling")
MODEL_FIELDS = ("gguf_sha256", "template_fix", "max_output_tokens", "reasoning_effort")


def preserved(before, after):
    return ({key: value for key, value in before.items() if key != "speed_percent"} ==
            {key: value for key, value in after.items() if key != "speed_percent"})


def check(label, turn, previous, trajectory, limits):
    kind = label["kind"]
    before, after = turn.get("before") or {}, turn.get("after") or {}
    x, y = before.get("speed_percent"), after.get("speed_percent")
    if (kind in PACE or kind == "restore") and turn.get("inferred_restart"):
        return None
    if kind == "restore":
        prior = (previous.get("before") or {}).get("speed_percent")
        if prior is None or x == prior:
            return None
    if not turn["first_pass"] or not turn["running"] or not after:
        return False
    if kind in PACE or kind == "restore":
        if not turn["was_running"] or x is None or y is None or not preserved(before, after):
            return False
        if x == y and turn["plan_changed"]:
            return False
        if kind == "hold":
            return x == y
        if kind == "up":
            return y > x or x == y == limits["speed_max_percent"]
        if kind == "down":
            return y < x or x == y == limits["speed_min_percent"]
        if kind == "restore":
            return y == prior
        if kind == "set":
            return y == label["value"]
        if kind == "adjust":
            return y == max(limits["speed_min_percent"], min(limits["speed_max_percent"], x + label["value"]))
        return y == limits["speed_max_percent"]
    if kind == "end_group":
        phrase = after.get("phrase") or {}
        groups = phrase.get("strokes") or phrase.get("regions") or []
        for index, group in enumerate(groups):
            at_end = group["bottom_percent"] >= 55 if label["end"] == "tip" else group["top_percent"] <= 50
            if at_end and group["count"] == label["count"]:
                if any(other["count"] == 1 and other["bottom_percent"] <= 10 and other["top_percent"] >= 90
                       for j, other in enumerate(groups) if j != index):
                    return True
        return False
    if not trajectory:
        return False
    lo, hi = trajectory["position_min_percent"], trajectory["position_max_percent"]
    if kind == "full_reach":
        return lo <= 2 and hi >= 98
    return hi - lo >= 10 and trajectory["mean_stroke_percent"] >= hi - lo - 2 and trajectory["stroke_length_cv"] <= .025


def score(capture, labels, split_after):
    turns, summaries = {}, {}
    current = {}
    labels_bound = isinstance(labels, dict)
    with capture.open(encoding="utf-8-sig") as stream:
        header = json.loads(next(stream))
        if header.get("schema") not in {"chat_history_replay.v1", "chat_history_replay.v2"}:
            raise ValueError("unrecognized replay schema")
        if labels_bound:
            if not labels.get("source_id") or labels["source_id"] != header.get("source_id"):
                raise ValueError("the annotations belong to a different source snapshot")
            labels = labels["labels"]
        for line in stream:
            row = json.loads(line)
            if row["kind"] != "turn":
                continue
            key = row["session"], row["step"]
            if key in turns:
                raise ValueError(f"duplicate capture turn {key}")
            turns[key] = row
            if row["plan_changed"] or not row["running"]:
                current[key[0]] = row.get("trajectory")
            summaries[key] = current.get(key[0])
    verdicts = defaultdict(dict)
    pace_turns, seen = set(), set()
    for label in labels:
        key = label["session"], label["step"]
        kind = label["kind"]
        if (key, kind) in seen or key not in turns:
            raise ValueError(f"duplicate label or missing capture turn {key} {kind}")
        if kind not in PACE | {"restore", "end_group", "full_reach", "full_pass"}:
            raise ValueError(f"unknown label {kind}")
        if kind == "end_group" and (label.get("end") not in {"base", "tip"} or label.get("count", 0) < 1):
            raise ValueError(f"invalid counted group at {key}")
        if kind in {"set", "adjust"} and type(label.get("value")) is not int:
            raise ValueError(f"expected a numeric pace annotation at {key}")
        if kind == "set" and not header["settings"]["speed_min_percent"] <= label["value"] <= header["settings"]["speed_max_percent"]:
            raise ValueError(f"requested set lies outside the saved speed limits at {key}")
        if kind == "adjust" and abs(label["value"]) > 100:
            raise ValueError(f"requested adjustment exceeds 100 points at {key}")
        seen.add((key, kind))
        groups = [kind]
        if kind in PACE:
            if key in pace_turns:
                raise ValueError(f"conflicting pace labels at {key}")
            pace_turns.add(key)
            groups.extend(["pace", "pace_late" if key[1] > split_after else "pace_early"])
        passed = check(label, turns[key], turns.get((key[0], key[1] - 1), {}), summaries[key], header["settings"])
        for group in groups:
            verdicts[group][key] = passed
    medians = {}
    for name, batch in [("pace", pace_turns), ("other", turns.keys() - pace_turns)]:
        if batch:
            medians[name] = {}
            for field in ("elapsed_ms", "generated_tokens", "prompt_tokens", "cached_tokens", "prefill_ms"):
                values = [turns[key][field] for key in batch if turns[key].get(field) is not None]
                if values:
                    medians[name][field] = statistics.median(values)
    scores = {}
    for kind, by_turn in verdicts.items():
        counts = Counter(by_turn.values())
        scores[kind] = {"passed": counts[True], "scored": counts[True] + counts[False], "unscorable": counts[None]}
    verdicts["first_pass"] = {key: row["first_pass"] for key, row in turns.items()}
    profile = {k: header[k] for k in (*PROFILE_FIELDS, *MODEL_FIELDS, "binary_sha256") if k in header}
    profile["labels_bound"] = labels_bound
    # Stop can finish without a provider. A generated turn missing its
    # sampling metadata makes the capture's sampler provenance incomplete.
    if turns and all(row.get("calls") == 0 or row.get("requested_sampling") is not None for row in turns.values()):
        sampling = sorted({json.dumps(row["requested_sampling"], sort_keys=True)
                           for row in turns.values() if row.get("requested_sampling") is not None})
        profile["requested_sampling"] = [json.loads(item) for item in sampling]
    report = {"turns": len(turns), "first_pass": sum(verdicts["first_pass"].values()),
              "scores": scores, "medians": medians, "profile": profile}
    if turns and all("outcome" in row for row in turns.values()):
        outcomes = Counter(row["outcome"] for row in turns.values())
        report["outcomes"] = dict(outcomes)
        report["completed"] = len(turns) - outcomes["failed"]
    return report, verdicts


def compare_profiles(before, after, allow_differences):
    if before.get("source_id") and after.get("source_id") and before["source_id"] != after["source_id"]:
        raise ValueError("session or message identities differ across captures")
    required = list(PROFILE_FIELDS)
    if "llama_cpp" in (before.get("provider"), after.get("provider")):
        required.extend(("gguf_sha256", "template_fix", "max_output_tokens"))
    if any(p.get("provider") and p["provider"] != "llama_cpp" for p in (before, after)):
        required.append("reasoning_effort")
    differences = [key for key in required if key not in before or key not in after or before[key] != after[key]]
    for key in ("source_id", "labels_bound"):
        if key not in differences and (not before.get(key) or not after.get(key)):
            differences.append(key)
    if differences and not allow_differences:
        raise ValueError(f"mismatched or unverified comparison fields ({', '.join(differences)}). "
                         "Use --allow-profile-differences for an intentional cross-profile or legacy comparison.")
    return differences


def paired_changes(before, after):
    if before["first_pass"].keys() != after["first_pass"].keys():
        raise ValueError("paired captures must contain the same sessions and steps")
    result = {}
    for group, checks in before.items():
        both = [(old, after[group][key]) for key, old in checks.items()
                if old is not None and after[group][key] is not None]
        result[group] = {"scored_in_both": len(both), "gained": sum(not old and new for old, new in both),
                         "lost": sum(old and not new for old, new in both)}
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("capture", type=Path)
    parser.add_argument("--labels", required=True, type=Path, help="Local JSON labels, optionally bound to a source_id")
    parser.add_argument("--against", type=Path, help="Baseline replay with the same session and turn ordinals")
    parser.add_argument("--allow-profile-differences", action="store_true", help="Report intentional differences or missing legacy metadata")
    parser.add_argument("--split-after", type=int, default=32)
    args = parser.parse_args()
    labels = json.loads(args.labels.read_text(encoding="utf-8-sig"))
    result, candidate = score(args.capture, labels, args.split_after)
    if args.against:
        baseline, previous = score(args.against, labels, args.split_after)
        differences = compare_profiles(baseline["profile"], result["profile"], args.allow_profile_differences)
        result = {"baseline": baseline, "candidate": result, "paired": paired_changes(previous, candidate),
                  "different_or_unverified": differences}
    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    main()
