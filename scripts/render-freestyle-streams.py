"""Render the opt-in Freestyle shared-plan matrix; no app or device access.

Usage: render-freestyle-streams.py compiled-matrix.json output-directory
Plot dependencies are in scripts/requirements-motion-atlas.txt.
"""

import html
import json
from pathlib import Path
import re
import sys

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
import numpy as np


source, destination = Path(sys.argv[1]), Path(sys.argv[2])
destination.mkdir(parents=True, exist_ok=True)
reports = json.loads(source.read_text(encoding="utf-8"))
groups = {}
cards = []
for name, report in sorted(reports.items()):
    profile, cap, seed, feel = name.split("/")
    key = re.sub(r"[^a-zA-Z0-9_-]", "-", name)
    groups.setdefault((profile, cap), []).append((name, key, report))
    x = np.asarray(report["samples"])
    v = np.asarray(report["velocities"])
    t = np.arange(len(x)) / report["rate_hz"]
    detail = t <= 12
    figure, axes = plt.subplots(2, 2, figsize=(14, 7))
    axes[0, 0].plot(t, x, color="#2b6cb0", linewidth=0.7)
    axes[0, 0].set(title="Whole stream", ylabel="Position %", xlabel="Seconds", ylim=(-3, 103))
    axes[0, 1].plot(t[detail], x[detail], color="#2b6cb0")
    axes[0, 1].set(title="Startup and individual turns", ylabel="Position %", xlabel="Seconds", ylim=(-3, 103))
    axes[1, 0].plot(t[detail], v[detail], color="#555555", linewidth=0.8)
    axes[1, 0].set(title="Exact planned velocity", ylabel="% / second", xlabel="Seconds")
    axes[1, 1].plot(x[detail], v[detail], color="#2b6cb0", linewidth=0.7)
    axes[1, 1].set(title="Phase portrait", xlabel="Position %", ylabel="% / second")
    figure.suptitle(f"{profile}; cap {cap}; seed {seed}; {feel}\n"
                   f"Shared plan, semantic 0–100, no transport. {report['retargets']} continuations; "
                   f"{report['blends']} blends. Peak a={report['peak_acceleration']:.0f}, "
                   f"j={report['peak_jerk']:.0f}.")
    figure.tight_layout()
    figure.savefig(destination / f"{key}.png", dpi=110)
    plt.close(figure)
    cards.append(f'<li><a href="{key}.png">{html.escape(name)}</a></li>')

overview_links = []
for (profile, cap), cases in sorted(groups.items()):
    figure, axes = plt.subplots(3, 3, figsize=(18, 10), sharex=True, sharey=True)
    for axis, (name, key, report) in zip(axes.flat, cases):
        x = np.asarray(report["samples"])
        t = np.arange(len(x)) / report["rate_hz"]
        axis.plot(t, x, color="#2b6cb0", linewidth=0.6)
        axis.set(title=name.rsplit("/", 2)[-2] + " / " + name.rsplit("/", 1)[-1], ylim=(-3, 103))
        axis.set_xlabel("Seconds")
    figure.suptitle(f"{profile}, speed cap {cap}: all three seeds and feels. Shared engine plans, no device telemetry.")
    figure.tight_layout()
    filename = f"overview-{profile}-{cap}.png"
    figure.savefig(destination / filename, dpi=100)
    plt.close(figure)
    overview_links.append(f'<a href="{filename}"><img src="{filename}" alt="{html.escape(profile)} cap {cap}"></a>')

(destination / "index.html").write_text(
    '<!doctype html><meta charset="utf-8"><title>Freestyle shared-engine review</title>'
    '<style>body{font:16px system-ui;margin:30px;color:#222}img{width:100%;max-width:1400px}'
    'a{color:#2b6cb0}li{margin:5px 0}</style><h1>Freestyle shared-engine review</h1>'
    '<p>Every evaluated profile, speed cap, seed and feel is retained. These are compiled semantic '
    'plans, not carriage telemetry. Live fake-transport captures are reviewed separately.</p>'
    + "".join(overview_links) + '<h2>Individual outputs</h2><ul>' + "".join(cards) + '</ul>',
    encoding="utf-8",
)
print(f"Rendered {len(reports)} detailed figures and {len(groups)} overview sheets to {destination}")
