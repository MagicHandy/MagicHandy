"""Compare multi-turn motion using the model and capability selected by setup.

The local profiles use the catalog GGUF in MAGICHANDY_QUALITY_MODEL_DIR and the
managed llama-server executable in MAGICHANDY_QUALITY_RUNNER. Terra uses the
signed-in MAGICHANDY_HOSTED_DATA_DIR. The optional external-local and compatible-local profiles use
MAGICHANDY_LIVE_LLAMA_URL, MAGICHANDY_LIVE_MODEL and MAGICHANDY_QUALITY_CAPABILITY.
All dispatches go to a test transport.
"""

import argparse
import json
from math import ceil
import os
from pathlib import Path
from statistics import median
import subprocess
import tempfile
from datetime import datetime, timezone

PROFILES = ("gemma-e4b", "gemma-12b", "terra")
ROOT = Path(__file__).resolve().parents[1]
TEST = "^TestLiveMotionRequestSequence$"


def run_profile(binary, scratch, output, profile, mirror, cases):
    variant = "b" if mirror else "a"
    name = f"{profile}-{variant}"
    pending = scratch / f"{name}.json"
    env = dict(os.environ, MAGICHANDY_QUALITY_PROFILE=profile,
               MAGICHANDY_QUALITY_MIRROR=str(int(mirror)),
               MAGICHANDY_EXPERIMENT_CAPTURE=str(pending))
    command = [str(binary), "-test.v", "-test.count=1", "-test.timeout=30m",
               f"-test.run={TEST}/^({cases or '.*'})$"]
    if mirror and not cases:
        # These eight fresh-session requests are identical in both variants.
        command.append(f"-test.skip={TEST}/^start_")
    log = output / f"{name}.log"
    with log.open("w", encoding="utf-8") as stream:
        result = subprocess.run(command, cwd=ROOT / "internal/httpapi", env=env,
                                stdout=stream, stderr=subprocess.STDOUT)
    if not pending.exists():
        print(f"{name}: no capture. See {log}", flush=True)
        return False
    capture = output / pending.name
    pending.replace(capture)
    report = json.loads(capture.read_bytes())
    turns = report["turns"]
    model_turns = [row for row in turns if row["expected_action"] != "stop"]
    stops = [row for row in turns if row["expected_action"] == "stop"]
    passed = sum(row["intent_pass"] for row in model_turns)
    holds = [row for row in turns if row["expected_action"] == "none"]
    unexpected = sum(row["events"]["action"] != "none" or
                     "unexpected change to playback" in (row.get("failures") or [])
                     for row in holds)
    repairs = sum(row["events"]["repaired"] or len(row.get("attempts") or []) > 1
                  for row in turns)
    print(f"{name}: {passed}/{len(model_turns)} first-pass model turns, {unexpected}/{len(holds)} "
          f"unexpected changes on holds, {repairs} turns retried, "
          f"{sum(row['intent_pass'] for row in stops)}/{len(stops)} direct Stops confirmed.")
    # A prompt rejected immediately is a quality failure. Its short round trip
    # would make a backend appear faster at delivering actual replies.
    elapsed = sorted(row["elapsed_ms"] for row in model_turns
                     if row["events"]["ok"] and not row["events"]["fallback"] and row["events"]["reply"].strip()
                     and any(attempt["raw"].strip() for attempt in row.get("attempts") or []))
    if elapsed:
        print(f"{name}: {len(elapsed)} delivered model replies, end-to-end median "
              f"{median(elapsed):.0f} ms, p95 {elapsed[ceil(len(elapsed) * .95) - 1]} ms.")
    print(capture, flush=True)
    return bool(turns) and result.returncode == 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profiles", nargs="+", choices=(*PROFILES, "external-local", "compatible-local"), default=PROFILES)
    parser.add_argument("--variants", nargs="+", choices=("a", "b"), default=("a", "b"),
                        help="Request wordings to run (default: both)")
    parser.add_argument("--cases", help="Go subtest expression, such as 'layered|restated'")
    parser.add_argument("--output", type=Path, help="Capture directory (default under .scratch)")
    args = parser.parse_args()
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    output = (args.output or ROOT / ".scratch" / "default-motion" / timestamp).resolve()
    output.mkdir(parents=True, exist_ok=True)
    passed = True
    with tempfile.TemporaryDirectory(dir=output) as folder:
        scratch = Path(folder)
        binary = scratch / ("motion-quality.exe" if os.name == "nt" else "motion-quality")
        subprocess.run(["go", "test", "-c", "-tags", "liveeval magichandy_labs",
                        "-o", str(binary), "./internal/httpapi"], cwd=ROOT, check=True)
        for profile in args.profiles:
            for variant in args.variants:
                if not run_profile(binary, scratch, output, profile, variant == "b", args.cases):
                    passed = False
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
