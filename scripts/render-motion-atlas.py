"""Render inert motion-atlas JSON with matplotlib; no app/device connection.

Each input has a manifest entry. Identical motion output shares a plot, while
requests/raw responses retain individual cards. Runtime artifacts stay in .scratch.
"""
import argparse
import hashlib
import html
import json
from datetime import datetime
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np

BLUE, GRAY = "#357bb7", "#737d88"


def render(entry, path):
    data = np.array(entry["samples"])
    seconds, position, velocity, acceleration = data.T
    wire = entry.get("wire", [])
    wt = np.array([p["time_ms"] / 1000 for p in wire])
    wp = np.array([p["position_percent"] for p in wire])
    fig = plt.figure(figsize=(14, 9), layout="constrained")
    grid = fig.add_gridspec(3, 2)
    whole = fig.add_subplot(grid[0, :])
    detail, phase = fig.add_subplot(grid[1, 0]), fig.add_subplot(grid[1, 1])
    speed, accel = fig.add_subplot(grid[2, 0]), fig.add_subplot(grid[2, 1])
    whole.plot(seconds, position, color=BLUE, lw=1.25)
    whole.set(title="Whole loop · commanded position", ylabel="Position (%)", ylim=(-3, 103))
    end = min(12, seconds[-1])
    chosen = seconds <= end
    detail.plot(seconds[chosen], position[chosen], color=BLUE, label="Planned")
    if wire:
        detail.plot(wt, wp, ".--", color=GRAY, lw=1, ms=3, label="Whole-percent wire points")
        slopes = np.diff(wp) / np.diff(wt)
        speed.stairs(slopes, wt, color=GRAY, lw=1, label="Wire segment velocity")
    detail.set(title="Initial excerpt · planned and wire", xlim=(0, end), ylim=(-3, 103), ylabel="Position (%)")
    detail.legend(fontsize=8)
    phase.plot(position, velocity, color=BLUE, lw=.7, alpha=.8)
    phase.set(title="Phase portrait · shape and directional balance", xlabel="Position (%)", ylabel="Velocity (%/s)", xlim=(-3, 103))
    speed.plot(seconds[chosen], velocity[chosen], color=BLUE, lw=1.3, label="Planned velocity")
    speed.set(title="Velocity in the initial excerpt", xlim=(0, end), xlabel="Seconds", ylabel="Velocity (%/s)")
    speed.legend(fontsize=8)
    accel.plot(seconds[chosen], acceleration[chosen], color=BLUE, lw=1)
    accel.set(title="Planned acceleration in the initial excerpt", xlim=(0, end), xlabel="Seconds", ylabel="Acceleration (%/s²)")
    for axis in (whole, detail, phase, speed, accel):
        axis.grid(alpha=.18)
        axis.spines[["top", "right"]].set_visible(False)
    summary = entry["summary"]
    caption = (f"{entry['name']} · requested {entry['speed_percent']}% · {entry['handy_model']}\n"
               f"mean {summary['commanded_mean_travel_percent_per_second']:.1f}%/s · peak {summary['commanded_peak_velocity_percent_per_second']:.1f}%/s · "
               f"acceleration {entry['peak_acceleration']:.0f}%/s² · finite-segment jerk {entry['peak_jerk']:.0f}%/s³ · span CV {summary['stroke_length_cv']:.3f}\n"
               f"Largest acceleration discontinuity at a knot: {entry.get('acceleration_jump', float('nan')):.2f}%/s²")
    fig.suptitle(caption, fontsize=13, fontweight="bold")
    fig.text(.5, -.015, "Commanded estimates; wire interpolation is not carriage feedback. Steady playback only; startup and retargeting have separate tests.", ha="center", fontsize=9)
    fig.savefig(path, dpi=130, bbox_inches="tight")
    plt.close(fig)


def contact_sheets(entries, destination):
    chosen, seen = [], set()
    for index, entry in enumerate(entries):
        if not entry.get("samples"):
            continue
        signature = hashlib.sha256(json.dumps(entry["samples"], separators=(",", ":")).encode()).hexdigest()
        if signature not in seen:
            chosen.append((index, entry))
            seen.add(signature)
    pages = []
    for start in range(0, len(chosen), 16):
        page = chosen[start:start + 16]
        fig, axes = plt.subplots(4, 4, figsize=(17, 11), layout="constrained")
        for axis, (index, entry) in zip(axes.flat, page):
            data = np.array(entry["samples"])
            axis.plot(data[:, 0], data[:, 1], color=BLUE, lw=.8)
            label = entry.get("request") or entry["name"]
            axis.set(title=f"{index+1}. {label[:52]}\n{entry['speed_percent']}% · {entry['handy_model']}", ylim=(-4, 104), xlabel="Seconds")
            axis.tick_params(labelsize=7)
            axis.title.set_fontsize(9)
            axis.grid(alpha=.15)
        for axis in list(axes.flat)[len(page):]:
            axis.set_visible(False)
        fig.suptitle("Motion character atlas · every distinct evaluated output · commanded position", fontsize=14)
        name = f"contact-{len(pages)+1:02d}.png"
        fig.savefig(destination / name, dpi=135)
        plt.close(fig)
        pages.append(name)
    return pages


def captured_transport_charts(paths, destination, start_index):
    """Render real captured test-dispatch points, including queued retargets.

    These are the transport interface's semantic floats, not rounded hardware
    commands. Points queued after Stop are explicitly shaded as canceled.
    """
    items = []
    for path in paths:
        source = json.loads(path.read_text(encoding="utf-8-sig"))
        speeds = []
        for turn in source.get("turns", []):
            speed = turn.get("motion", {}).get("speed_percent")
            if speed is not None and (not speeds or speed != speeds[-1]):
                speeds.append(speed)
        speed_label = source.get("speed", " → ".join(map(str, speeds)) or "not recorded")
        streams, plays, stops = {}, {}, []
        for command in source.get("commands", []):
            if command["kind"] == "points_add":
                batch = command["points_add"]
                streams.setdefault(batch["stream_id"], []).extend(batch["points"])
            elif command["kind"] == "points_play":
                plays[command["points_play"]["stream_id"]] = datetime.fromisoformat(command["issued_at"])
            elif command["kind"] == "stop":
                stops.append(datetime.fromisoformat(command["issued_at"]))
        for stream, points in streams.items():
            seconds = np.array([p["time_ms"]/1000 for p in points])
            positions = np.array([p["position_percent"] for p in points])
            if np.any(np.diff(seconds) <= 0):
                raise ValueError(f"Nonmonotonic captured stream: {stream}")
            velocities = np.diff(positions)/np.diff(seconds)
            fig, axes = plt.subplots(2, 1, figsize=(14, 8), layout="constrained", sharex=True)
            axes[0].plot(seconds, positions, ".-", color=BLUE, ms=4)
            axes[0].set(ylabel="Semantic position (%)", ylim=(-3, 103))
            axes[1].stairs(velocities, seconds, color=BLUE)
            axes[1].set(xlabel="Stream seconds", ylabel="Segment velocity (%/s)")
            stop = next((time for time in stops if stream in plays and time >= plays[stream]), None)
            if stop and stream in plays:
                stop_at = (stop-plays[stream]).total_seconds()
                for axis in axes:
                    axis.axvspan(stop_at, seconds[-1], color=GRAY, alpha=.16)
                    axis.axvline(stop_at, color=GRAY, linestyle="--", label="Stop issued; queued remainder canceled")
                axes[0].legend(loc="upper left")
            for axis in axes:
                axis.grid(alpha=.18)
            scenario = source.get("scenario", "Full sweeps → tip-anchored variety → pace edit → Stop")
            fig.suptitle(f"App → shared engine → captured test transport\n{scenario}", fontsize=14)
            fig.text(.5, -.025, "Actual queued semantic samples from a fake transport. No physical device; no inferred acceleration or carriage feedback.", ha="center")
            name = f"captured-{len(items)+1:02d}.png"
            fig.savefig(destination/name, dpi=130, bbox_inches="tight")
            plt.close(fig)
            items.append({"index": start_index+len(items)+1, "id": stream, "name": "Captured app motion timeline", "group": "llm-output", "request": " / ".join(t.get("request", t.get("message", "")) for t in source.get("turns", [])), "raw": "", "model": source.get("model", ""), "error": "", "outcome": "Actual test dispatch, including queued transitions and Stop", "speed": speed_label, "plot": name})
    return items


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("input", type=Path)
    parser.add_argument("output", type=Path)
    parser.add_argument("--captured", type=Path, action="append", default=[], help="Optional app-to-fake-transport live test JSON (repeatable)")
    args = parser.parse_args()
    source = json.loads(args.input.read_text(encoding="utf-8-sig"))
    entries = source["entries"]
    args.output.mkdir(parents=True, exist_ok=True)
    seen, manifest = {}, []
    for index, entry in enumerate(entries):
        item = {"index": index+1, "id": entry["id"], "name": entry["name"], "group": entry["group"], "request": entry.get("request", ""), "raw": entry.get("raw", ""), "model": entry.get("model", ""), "error": entry.get("error", ""), "outcome": entry.get("outcome", ""), "speed": entry["speed_percent"]}
        if entry.get("samples"):
            signature = hashlib.sha256(json.dumps([entry["samples"], entry["wire"]], separators=(",", ":")).encode()).hexdigest()[:16]
            if signature not in seen:
                seen[signature] = f"motion-{signature}.png"
                if not (args.output / seen[signature]).exists():
                    render(entry, args.output / seen[signature])
            item["plot"] = seen[signature]
        manifest.append(item)
    pages = contact_sheets(entries, args.output)
    manifest.extend(captured_transport_charts(args.captured, args.output, len(manifest)))
    cards = []
    for item in manifest:
        esc = lambda value: html.escape(str(value))
        figure = f'<a href="{item["plot"]}"><img loading="lazy" src="{item["plot"]}" alt="Motion plots"></a>' if "plot" in item else ""
        details = f'<details><summary>Raw model output</summary><pre>{esc(item["raw"])}</pre></details>' if item["raw"] else ""
        speed_label = f' · {esc(item["speed"])}%' if item["speed"] != "not recorded" else ""
        cards.append(f'<article data-group="{item["group"]}"><h2>{item["index"]}. {esc(item["name"])}{speed_label}</h2><p>{esc(item["id"])} · {esc(item["group"])} · {esc(item["model"])}</p><p>{esc(item["request"])}</p><p>{esc(item["outcome"])} {esc(item["error"])}</p>{figure}{details}</article>')
    overview = "".join(f'<a href="{name}"><img src="{name}" alt="Library contact sheet"></a>' for name in pages)
    document = f'''<!doctype html><html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Motion character atlas</title>
<style>body{{font:16px/1.5 system-ui;margin:24px auto;max-width:1500px;padding:0 20px;color:#222;background:#f7f8fa}}img{{max-width:100%;height:auto}}article{{padding:24px;margin:24px 0;background:white;border:1px solid #ccd1d7}}pre{{white-space:pre-wrap;overflow-wrap:anywhere}}nav{{position:sticky;top:0;background:#f7f8fa;padding:12px;border-bottom:1px solid #ccd1d7}}button{{padding:8px 14px;margin:4px;border:1px solid #357bb7;cursor:pointer}}h2{{font-size:20px}}</style>
<h1>Motion character atlas</h1><p>{len(entries)} steady motion cases, {len(seen)} distinct rendered steady outputs, {len(manifest)-len(entries)} captured timelines. Every model reply has a record; rejected replies and Stop have no moving plot. Identical outputs share a figure. These are commanded estimates, not physical feedback.</p>
<details open><summary>Overview of all distinct evaluated outputs</summary>{overview}</details>
<nav><label>Filter <input id="query" placeholder="Name, request or model"></label><button data-filter="flow-experiments">Flow experiments</button><button data-filter="new-library">New library</button><button data-filter="legacy-library">Legacy library</button><button data-filter="llm-output">LLM outputs</button><button data-filter="">All cases</button></nav>{''.join(cards)}
<script>let group=document.querySelector('article')?.dataset.group||'';function filter(){{const q=document.querySelector('#query').value.toLowerCase();document.querySelectorAll('article').forEach(a=>a.hidden=(group&&a.dataset.group!==group)||!a.textContent.toLowerCase().includes(q))}}document.querySelectorAll('button').forEach(b=>b.onclick=()=>{{group=b.dataset.filter;filter()}});document.querySelector('#query').oninput=filter;filter();</script></html>'''
    (args.output / "index.html").write_text(document, encoding="utf-8")
    (args.output / "manifest.json").write_text(json.dumps(manifest, indent=2), encoding="utf-8")
    print(f"Rendered {len(entries)} cases to {len(seen)} plots and {len(pages)} overview sheets: {args.output / 'index.html'}")


if __name__ == "__main__":
    main()
