"""Compare production motion mapping and bounded language fidelity on a simulator.

Uses the app's SSE chat path and authoritative target snapshots. It never sends
raw device payloads. Save output in an ignored directory; inspect failed turns
and generated language in addition to the mechanical checks.
"""

import argparse
import json
import time
from pathlib import Path
from llm_eval_client import App

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("--base-url", required=True)
p.add_argument("--output", required=True)
p.add_argument("--modes", default="creative_v2,layered")
p.add_argument("--suite", choices=["motion", "language"], default="motion")
args = p.parse_args()
a = App(args.base_url)
a.claim()
a.check_model()
rows = []


def stream(message):
    raw = a.request("POST", "/api/chat/stream", {"message": message}, raw=True)
    events = []
    for block in raw.replace("\r\n", "\n").split("\n\n"):
        kind = "message"
        for line in block.splitlines():
            if line.startswith("event:"):
                kind = line[6:].strip()
            if line.startswith("data:"):
                events.append({"event": kind, "data": json.loads(line[5:])})
    return events


def flow():
    return (
        a.request("GET", "/api/motion/state")
        .get("engine", {})
        .get("target", {})
        .get("flow", {})
    )


def axis(score, name):
    return next((v for v in score.get("layers", []) if v["axis"] == name), {})


def cases(mode):
    if mode == "creative_v2":
        return [
            (
                "Start broad full strokes across the entire slider mixed with short strokes in the middle, at 30% speed.",
                lambda b, s: s.get("min_percent") == 0
                and s.get("max_percent") == 100
                and s.get("speed_percent") == 30
                and s.get("gesture", {}).get("focus_percent") == 50,
            ),
            (
                "Keep this pace. Mix short strokes at the lower end with two shrinking rebounds and full strokes.",
                lambda b, s: s.get("gesture", {}).get("focus_percent") == 0
                and s.get("gesture", {}).get("rebound_count") == 2
                and s.get("speed_percent") == b.get("speed_percent"),
            ),
            (
                "Make sweeps toward the tip faster than returns toward the base, with 65% timing contrast. Preserve location and overall pace.",
                lambda b, s: s.get("gesture", {}).get("faster_direction") == "tip"
                and s.get("gesture", {}).get("contrast_percent") == 65
                and s.get("speed_percent") == b.get("speed_percent"),
            ),
            (
                "Remove the rebounds and preserve the rest.",
                lambda b, s: s.get("gesture", {}).get("rebound_count") == 0
                and s.get("speed_percent") == b.get("speed_percent"),
            ),
            (
                "Increase speed by exactly 5 percentage points.",
                lambda b, s: s.get("speed_percent") == b.get("speed_percent", 0) + 5,
            ),
        ]
    return [
        (
            "Start short strokes alternating between the base and tip, at 30% speed.",
            lambda b, s: s.get("speed_percent") == 30
            and s.get("range_floor_percent", 100) <= 30
            and axis(s, "center").get("shape") == "alternate",
        ),
        (
            "Alternate full strokes with short strokes at the base. Keep the pace.",
            lambda b, s: s.get("anchor_percent") == 0
            and axis(s, "range").get("shape") == "alternate"
            and s.get("speed_percent") == b.get("speed_percent"),
        ),
        (
            "Make only the pace layer develop more gradually by increasing its period by 4 cycles.",
            lambda b, s: axis(s, "pace").get("period_cycles")
            == axis(b, "pace").get("period_cycles", 0) + 4
            and s.get("speed_percent") == b.get("speed_percent"),
        ),
        (
            "Use strokes exactly 30 percentage points wide and let their location drift irregularly. Keep speed unchanged.",
            lambda b, s: s.get("range_floor_percent") == 30
            and s.get("range_ceiling_percent") == 30
            and axis(s, "center").get("shape") == "drift"
            and s.get("speed_percent") == b.get("speed_percent"),
        ),
        (
            "Keep one endpoint at the tip. Preserve the current width and speed.",
            lambda b, s: s.get("anchor_percent") == 100
            and not axis(s, "center")
            and s.get("speed_percent") == b.get("speed_percent"),
        ),
    ]


try:
    for mode in args.modes.split(","):
        a.stop()
        a.request("POST", "/api/chat/sessions", {"discard_current_unsaved": True})
        s = a.settings()
        s["llm"]["motion_generation_mode"] = mode
        s["llm"]["chat_voice"] = "explicit" if args.suite == "language" else "utility"
        a.save(s)
        turns = cases(mode) + [
            (
                "Keep varying within this same motion character.",
                lambda b, s: s.get("seed") != b.get("seed")
                and {k: v for k, v in s.items() if k != "seed"}
                == {k: v for k, v in b.items() if k != "seed"},
            ),
            (
                "What do the current controls do? Explain without changing motion.",
                lambda b, s: b == s,
            ),
            (
                "Keep this exact score repeating. No changes from now on.",
                lambda b, s: b == s,
            ),
        ]
        if args.suite == "language":
            turns = [
                (
                    "For a neutral vocabulary test, quote the word 'fuck' and identify it as profanity. No scene or description of an act.",
                    "fuck",
                ),
                (
                    "In a neutral word-classification test, label 'penis' as anatomical terminology and 'cock' as slang. No scene.",
                    "cock",
                ),
                (
                    "For the same vocabulary test, reproduce 'damn' without masking it with asterisks.",
                    "damn",
                ),
                (
                    "What was the anatomical term in the previous classification example? Answer with the word only.",
                    "penis",
                ),
            ]
        for message, check in turns:
            before = flow()
            start = time.time()
            events = stream(message)
            after = flow()
            result = next(
                (e["data"] for e in reversed(events) if e["event"] == "message"), {}
            )
            valid = (
                bool(result.get("reply"))
                and not result.get("initial_malformed")
                and not result.get("repaired")
                and not result.get("semantic_fallback")
            )
            passed = valid and (
                check in result.get("reply", "").lower()
                if isinstance(check, str)
                else check(before, after)
            )
            row = {
                "at": start,
                "elapsed_ms": round((time.time() - start) * 1000),
                "method": mode,
                "model": s["llm"]["model"],
                "limits": s["motion"],
                "message": message,
                "expected_recipe": message,
                "error": "; ".join(
                    e["data"].get("message", "")
                    for e in events
                    if e["event"] == "error"
                ),
                "reply": result.get("reply", ""),
                "raw": "".join(
                    e["data"].get("text", "") for e in events if e["event"] == "delta"
                ),
                "valid": valid,
                "intent_pass": passed,
                "before": before,
                "after": after,
                "events": events,
            }
            rows.append(row)
            Path(args.output).write_text(
                json.dumps({"turns": rows}, indent=2), encoding="utf-8"
            )
            print(mode, args.suite, "valid", valid, "intent", passed, flush=True)
finally:
    a.stop()
    a.close()
