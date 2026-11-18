"""Replay installed chat through an isolated app, a default model and a fake transport.

Select --profile terra to send original user turns and recorded Autopilot remarks
to the signed-in ChatGPT provider. Gemma profiles run locally. The result contains
ordinal positions, timings and commanded scores. Original text stays out of it.
"""

import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import sqlite3
import statistics
import subprocess
import tempfile


ROOT = Path(__file__).resolve().parents[1]


def read_conversations(database, selected):
    connection = sqlite3.connect(database.resolve().as_uri() + "?mode=ro", uri=True)
    try:
        connection.execute("PRAGMA query_only=ON")
        connection.execute("BEGIN")
        document, = connection.execute("SELECT document FROM settings").fetchone()
        limits = json.loads(document)["motion"]
        sessions = []
        for number, (session_id,) in enumerate(connection.execute(
                "SELECT id FROM chat_sessions ORDER BY created_at, id"), 1):
            if selected and number not in selected:
                continue
            events = []
            pending_user = None
            started = False
            rows = connection.execute(
                "SELECT role, content, diagnostics_json FROM messages "
                "WHERE session_id=? AND committed=1 ORDER BY seq", (session_id,))
            for role, content, diagnostics in rows:
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
                    # A later recorded start implies an intervening stop, which
                    # the persisted transcript does not otherwise timestamp.
                    action = info.get("motion_action")
                    if action == "start":
                        pending_user["reset"] = started
                        started = True
                    elif action == "stop":
                        started = False
                    pending_user = None
            if any(event["kind"] == "user" for event in events):
                sessions.append({"number": number, "events": events})
        if not sessions:
            raise ValueError("the selection contains no retained user turns")
        return {"min": limits["speed_min_percent"], "max": limits["speed_max_percent"],
                "sessions": sessions}
    finally:
        connection.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--database", type=Path, required=True, help="Installed magichandy.db, opened read-only")
    parser.add_argument("--profile", required=True, choices=("gemma-12b", "gemma-e4b", "terra"))
    parser.add_argument("--sessions", type=int, nargs="+", help="Session ordinals in creation order")
    parser.add_argument("--output", type=Path, help="New JSONL file, defaults under .scratch")
    args = parser.parse_args()
    transcript = read_conversations(args.database, set(args.sessions or ()))
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
    print(f"{first}/{len(turns)} first-pass completions, median {median:.0f} ms. {output}")
    return 0 if first == len(turns) else 1


if __name__ == "__main__":
    raise SystemExit(main())
