"""Render engine-sampled native tempo captures; artifacts stay outside git.

Usage: python scripts/render-tempo-review.py path/to/atlas.json output-directory
Uses the existing optional matplotlib dependency from requirements-motion-atlas.
"""
import hashlib
import html
import json
import sys
from pathlib import Path

import matplotlib
matplotlib.use("Agg")
import matplotlib.pyplot as plt


def outcome(row):
    if "intent_pass" in row:
        return "PASS" if row["intent_pass"] else "FAILED INTENT"
    return row.get("review_status", "UNSCORED").upper()


def main():
    source, output = Path(sys.argv[1]), Path(sys.argv[2])
    output.mkdir(parents=True, exist_ok=True)
    atlas = json.loads(source.read_text(encoding="utf-8"))
    unique = {}
    for row in atlas["rows"]:
        levels = row.get("levels")
        if levels is None:
            continue
        key = hashlib.sha256(json.dumps(levels).encode()).hexdigest()[:12]
        row["plot"] = f"{key}.png"
        unique.setdefault(key, row)
    for key, row in unique.items():
        fig, axes = plt.subplots(2, 1, figsize=(11, 6), constrained_layout=True)
        levels = row["levels"]
        times = [i * atlas["sample_ms"] / 1000 for i in range(len(levels))]
        for axis in axes:
            axis.plot(times, levels, color="#427b9f", linewidth=1.7)
            axis.set(ylim=(-2, 102), ylabel="Tempo (% allowed range)")
            axis.grid(alpha=.2)
        status = outcome(row)
        axes[0].set_title(f"{status}: {row['name']} | {row['model']} | seed {row.get('seed', '')}")
        axes[1].set(xlim=(0, 15), xlabel="Seconds (engine clock)")
        axes[1].set_title("Entry and first phrase")
        fig.savefig(output / f"{key}.png", dpi=110)
        plt.close(fig)
    plots = list(unique.values())
    for offset in range(0, len(plots), 20):
        fig, axes = plt.subplots(5, 4, figsize=(18, 16), constrained_layout=True)
        for axis, row in zip(axes.flat, plots[offset:offset+20]):
            axis.plot([i*.1 for i in range(len(row["levels"]))], row["levels"], color="#427b9f")
            title = row["name"].split("/")[-1]
            if row.get("intent_pass") is False:
                title = "FAILED: " + title
            axis.set(ylim=(-2, 102), title=title)
            axis.grid(alpha=.2)
        for axis in list(axes.flat)[len(plots[offset:offset+20]):]:
            axis.axis("off")
        fig.savefig(output / f"overview-{offset//20+1}.png", dpi=110)
        plt.close(fig)
    cards = []
    for row in atlas["rows"]:
        plot = f'<img loading="lazy" src="{row["plot"]}" alt="Engine tempo plot">' if "plot" in row else '<p>No new tempo was accepted.</p>'
        evidence = {key: value for key, value in row.items() if key not in ("levels",)}
        status = outcome(row)
        cards.append(f'<article><h2>{html.escape(row["name"])} — {status}</h2>{plot}<details><summary>Raw evidence</summary><pre>{html.escape(json.dumps(evidence, indent=2))}</pre></details></article>')
    (output / "index.html").write_text('<!doctype html><meta charset="utf-8"><title>Native tempo review</title><style>body{font:16px system-ui;background:#f3f4f5;color:#18212a;margin:24px}article{border-top:1px solid #bbc2c9;padding:18px}img{width:min(100%,1000px)}pre{white-space:pre-wrap}</style><h1>VacuGlide native tempo — engine review</h1><p>Includes failed selections. Graphs show planned motor tempo, not measured stroke position or physical smoothness.</p>'+"".join(cards),encoding="utf-8")
    (output / "manifest.json").write_text(json.dumps({"records":len(atlas["rows"]),"distinct_plots":len(plots),"results":[{k:v for k,v in row.items() if k!="levels"} for row in atlas["rows"]]},indent=2),encoding="utf-8")
    print(f"{len(atlas['rows'])} records, {len(plots)} distinct plots, {(len(plots)+19)//20} overview sheets")


if __name__ == "__main__":
    main()
