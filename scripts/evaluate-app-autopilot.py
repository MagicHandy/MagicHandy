"""Capture actual full-app Autopilot with no human motion requests.

Requires a disposable app started with -simulate-motion. Each mode gets a new
chat session. Accepted targets, failed decisions, speech, samples and Stop are
retained for visual and semantic review; this does not measure physical feel.
"""

import argparse, json, time
from pathlib import Path
from llm_eval_client import App

p = argparse.ArgumentParser(description=__doc__)
p.add_argument("--base-url", required=True)
p.add_argument("--output", required=True)
p.add_argument("--seconds", type=int, default=180)
p.add_argument("--modes", default="creative_v2,layered,dynamic")
p.add_argument("--voice", default="utility")
args = p.parse_args()
a = App(args.base_url)
a.claim()
a.check_model()
sessions = []
try:
    status = a.request("GET", "/api/state")
    assert status.get(
        "motion_simulated"
    ), "This harness requires the full build started with -simulate-motion."
    for mode in args.modes.split(","):
        a.stop()
        a.request("POST", "/api/chat/sessions", {"discard_current_unsaved": True})
        s = a.settings()
        s["llm"]["motion_generation_mode"] = mode
        s["llm"]["chat_voice"] = args.voice
        s["autopilot"].update(
            motion_change_level=8,
            speech_cadence="custom",
            speech_min_seconds=8,
            speech_max_seconds=12,
            speech_motion_authority="chat_only",
            session_tracking=True,
        )
        a.save(s)
        run = {
            "mode": mode,
            "model": s["llm"]["model"],
            "settings": s["motion"],
            "seconds": args.seconds,
            "targets": [],
            "samples": [],
            "messages": [],
            "events": [],
        }
        sessions.append(run)
        a.request("POST", "/api/modes/start", {"mode": "autopilot"})
        start = time.monotonic()
        last_plan = ""
        last_message = 0
        last_mode = ""
        while time.monotonic() - start < args.seconds:
            time.sleep(0.3)
            snap = a.request("GET", "/api/motion/state")
            engine = snap.get("engine", {})
            seconds = round(time.monotonic() - start, 3)
            if engine.get("plan_id") and engine["plan_id"] != last_plan:
                last_plan = engine["plan_id"]
                run["targets"].append(
                    {
                        "at": seconds,
                        "target": engine["target"],
                        "pace": engine.get("pace"),
                        "plan_id": last_plan,
                    }
                )
            if engine.get("current_sample"):
                run["samples"].append(
                    {"at": seconds, "sample": engine["current_sample"]}
                )
            mode_state = a.request("GET", "/api/modes")
            serialized = json.dumps(
                {
                    k: v
                    for k, v in mode_state.items()
                    if k
                    in [
                        "last_error",
                        "decision_source",
                        "segment_index",
                        "last_say",
                        "last_decision_ms",
                        "last_decision_error",
                    ]
                },
                sort_keys=True,
            )
            if serialized != last_mode:
                run["events"].append({"at": seconds, "state": mode_state})
                last_mode = serialized
            messages = a.request("GET", f"/api/chat/messages?after={last_message}").get(
                "messages", []
            )
            for msg in messages:
                if msg["seq"] > last_message:
                    run["messages"].append(
                        {
                            "at": seconds,
                            "message": msg,
                            "target": engine.get("target"),
                            "pace": engine.get("pace"),
                        }
                    )
                    last_message = msg["seq"]
        a.stop()
        run["trace"] = a.request("GET", "/api/traces")
        run["stopped"] = (
            not a.request("GET", "/api/motion/state")
            .get("engine", {})
            .get("running", False)
        )
        print(
            f'{mode}: {len(run["targets"])} accepted targets, {len(run["messages"])} chat lines, stopped={run["stopped"]}',
            flush=True,
        )
        Path(args.output).write_text(
            json.dumps({"sessions": sessions}, indent=2), encoding="utf-8"
        )
finally:
    a.stop()
    a.close()
    Path(args.output).write_text(
        json.dumps({"sessions": sessions}, indent=2), encoding="utf-8"
    )
