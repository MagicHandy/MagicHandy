"""Compare multi-turn motion using the model and capability selected by setup.

The local profiles use the catalog GGUF in MAGICHANDY_QUALITY_MODEL_DIR and the
managed llama-server executable in MAGICHANDY_QUALITY_RUNNER. Terra uses the
signed-in MAGICHANDY_HOSTED_DATA_DIR. All dispatches go to a test transport.
"""

import argparse
import json
import os
from pathlib import Path
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
    passed = sum(row["intent_pass"] for row in turns)
    holds = [row for row in turns if row["expected_action"] == "none"]
    unexpected = sum(row["events"]["action"] != "none" or
                     "unexpected change to playback" in (row.get("failures") or [])
                     for row in holds)
    repairs = sum(row["events"]["repaired"] or len(row.get("attempts") or []) > 1
                  for row in turns)
    print(f"{name}: {passed}/{len(turns)} first-pass turns, {unexpected}/{len(holds)} "
          f"unexpected changes on holds, {repairs} turns retried. {capture}", flush=True)
    return bool(turns) and result.returncode == 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--profiles", nargs="+", choices=PROFILES, default=PROFILES)
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
            for mirror in (False, True):
                if not run_profile(binary, scratch, output, profile, mirror, args.cases):
                    passed = False
    return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
