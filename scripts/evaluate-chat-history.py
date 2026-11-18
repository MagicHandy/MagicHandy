"""Replay installed chat through an isolated app, a default model and a fake transport.

Select --profile terra to send original user turns and recorded Autopilot remarks
to the signed-in ChatGPT provider. Gemma profiles run locally. The result contains
ordinal positions, timings and commanded scores. Original text stays out of it.
"""

import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import sqlite3
import statistics
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]


def replay_calibration(settings):
    motion = settings.get("motion", {})
    fields = ("speed_min_percent", "speed_max_percent", "stroke_min_percent", "stroke_max_percent",
              "reverse_direction", "apply_video_speed_limit", "style", "handy_model")
    device = settings.get("device", {})
    models = {item["id"]: item.get("handy_model") for item in device.get("handy_connections") or [] if item.get("id")}
    models["handy"] = motion.get("handy_model")
    # Legacy playback sets select at most the primary Handy. Its calibration
    # is already in motion. No connection IDs or credentials leave this reader.
    active_models = [models[key] for key in device.get("playback_devices") or [] if models.get(key)]
    return {"motion": {key: motion[key] for key in fields if key in motion}, "handy_models": active_models}


def read_conversations(database, selected, *, infer_stops=True):
    connection = sqlite3.connect(database.resolve().as_uri() + "?mode=ro", uri=True)
    try:
        connection.execute("PRAGMA query_only=ON")
        connection.execute("BEGIN")
        document, = connection.execute("SELECT document FROM settings").fetchone()
        calibration = replay_calibration(json.loads(document))
        sessions = []
        identity = hashlib.sha256()
        for number, (session_id,) in enumerate(connection.execute(
                "SELECT id FROM chat_sessions ORDER BY created_at, id"), 1):
            if selected and number not in selected:
                continue
            events = []
            session_identity = hashlib.sha256(str(session_id).encode())
            pending_user = None
            started = False
            rows = connection.execute(
                "SELECT seq, role, content, diagnostics_json FROM messages "
                "WHERE session_id=? AND committed=1 ORDER BY seq", (session_id,))
            for seq, role, content, diagnostics in rows:
                # Only opaque row identities contribute to the comparison ID.
                # Neither prose nor account/device settings are fingerprinted.
                session_identity.update(f"\n{seq}/{role}".encode())
                if role == "user":
                    pending_user = {"kind": "user", "content": content}
                    events.append(pending_user)
                    continue
                if role != "assistant":
                    continue
                info = json.loads(diagnostics or "{}") or {}
                if info.get("source") == "autopilot":
                    events.append({"kind": "autopilot", "content": content})
                elif pending_user is not None:
                    # Diagnostics record the proposed action before dispatch.
                    # A later start is evidence of a gap, not an acknowledged Stop.
                    action = info.get("motion_action")
                    if action == "start":
                        pending_user["reset"] = infer_stops and started
                        started = True
                    elif action == "stop":
                        started = False
                    pending_user = None
            if any(event["kind"] == "user" for event in events):
                sessions.append({"number": number, "events": events})
                identity.update(f"{number}/".encode() + session_identity.digest())
        if not sessions:
            raise ValueError("the selection contains no retained user turns")
        return {**calibration, "sessions": sessions, "source_id": identity.hexdigest(), "infer_stops": infer_stops}
    finally:
        connection.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", type=Path, required=True, help="Installed magichandy.db, opened read-only")
    parser.add_argument("--profile", required=True, choices=("gemma-12b", "gemma-e4b", "terra"))
    parser.add_argument("--sessions", type=int, nargs="+", help="Session ordinals in creation order")
    parser.add_argument("--no-infer-stops", action="store_true",
                        help="Replay consecutive start proposals without inserting a simulated Stop")
    parser.add_argument("--output", type=Path, help="New JSONL file, defaults under .scratch")
    args = parser.parse_args()
    transcript = read_conversations(args.database, set(args.sessions or ()), infer_stops=not args.no_infer_stops)
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    output = (args.output or ROOT / ".scratch" / "chat-history" / f"{stamp}-{args.profile}.jsonl").resolve()
    output.parent.mkdir(parents=True, exist_ok=True)
    if output.exists():
        parser.error("the output file already exists")
    env = dict(os.environ, MAGICHANDY_QUALITY_PROFILE=args.profile, MAGICHANDY_REPLAY_OUTPUT=str(output))
    with tempfile.TemporaryDirectory(dir=output.parent) as directory:
        binary = Path(directory) / ("replay.exe" if os.name == "nt" else "replay")
        subprocess.run(["go", "test", "-c", "-tags", "liveeval magichandy_labs",
                        "-o", str(binary), "./internal/httpapi"], cwd=ROOT, check=True)
        subprocess.run([str(binary), "-test.run=^TestLiveChatHistoryReplay$", "-test.count=1",
                        "-test.timeout=0"], cwd=ROOT / "internal/httpapi", env=env,
                       input=json.dumps(transcript).encode(), check=True)
    with output.open(encoding="utf-8") as stream:
        turns = [row for line in stream if (row := json.loads(line))["kind"] == "turn"]
    first = sum(row["first_pass"] for row in turns)
    median = statistics.median(row["elapsed_ms"] for row in turns)
    outcomes = Counter(row["outcome"] for row in turns)
    completed = len(turns) - outcomes["failed"]
    detail = ", ".join(f"{key}={value}" for key, value in sorted(outcomes.items()))
    print(f"{first}/{len(turns)} first pass, {completed}/{len(turns)} completed, "
          f"median {median:.0f} ms ({detail}). {output}")
    return 0 if first == len(turns) else 1


if __name__ == "__main__":
    raise SystemExit(main())
