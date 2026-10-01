# Freestyle stream implementation and review — 2026-10-01

The interrupted Freestyle draft has been completed for PR review. The older
intent was continuous motion that develops without recurring transport starts,
with customization independent of the pattern catalog. The implementation is
described in [ADR 0038](decisions/0038-freestyle-stroke-stream.md).

## Review findings

| Priority | Reproduction | Resolution |
| --- | --- | --- |
| P1 | A settled ramp was compacted before the playhead, although neighboring timing and event blocks still read its earlier controls. Seed 1 at stroke 23 changed its lower turn from 81.846 to 62.333. | Retain the full surrounding timing and event-control context. Exact overlapping-stroke and compaction regressions cover multiple seeds and window starts. |
| P2 | Select Intense, then edit Focus while the preset write is pending. The next Custom write restores Balanced pace, length and variety. | Serialize writes and merge only queued edited fields onto the acknowledged backend preset. |
| P2 | Choose a one-minute Cooldown after more than a minute of Steady motion, including a preference update between the tick's clock and planning reads. It immediately reports finished and queues the end. | Each planning snapshot reconciles its selected shape with a fresh active-time arc. A deterministic regression forces the update between reads. A completed wind-down remains terminal and reports its applied shape honestly. |
| P2 | Older settings with Gentle or Intense would acquire Balanced Freestyle preferences. | Preserve the previously saved motion style when introducing the new section. |
| Gate | Missing route admission entry, seven lint findings and an oversized API types file. | Add the controller-only policy, focused validation/test helpers and a separate Freestyle types file. Keep all gates enabled. |
| Test | Linux race CI sometimes observed Cooldown's inactive status before its completion trace was appended after loop teardown. | Wait separately for the completion trace; both assertions remain required, with bounded waits. Production lifecycle ordering is unchanged. |

The full race run also reproduced the stalled-content timeout already reviewed
in PR #290. This branch carries its test-only bound fix from `050be97d`: socket
buffers must first fill before the five-second write deadline starts. The
twenty-second outer wait still fails a handler that never releases capacity;
production deadlines and route admission are unchanged.

## Numerical and visual evidence

Local artifacts are under `.scratch/freestyle-review-20261001/`, excluded from
Git. `compiled-matrix.json` contains all 81 evaluated combinations: Original,
2 Standard and 2 Pro; caps 10/43/100; seeds 1/17/24301; and all three feels.
The shared compiler generated every output. Compilation took 1.499–11.113 ms
(median 3.004 ms in this run). The atlas retains 81 detailed figures, all nine
overview sheets and a local `atlas/index.html`; there were zero continuation
blends. No rejected outputs are omitted.

All overview sheets were inspected, followed by detailed seed-17 figures for
each feel at 10/43/100, plus the largest sampled acceleration outliers on
Original and 2 Pro. Gentle has long, calmer turns; Balanced develops reach
and pace; Intense favors shorter, quicker work, with occasional wider strokes
and brief rests. Startup eases from rest. The phase portraits and exact planned
velocity show rounded reversals, without extra reversals at window boundaries.
Sparse wide-stroke events at high variety remain a deliberate character
choice, requiring physical feedback rather than a claim of perfect feel.

The final real-time captures use the shared engine and a fake transport, with
Original limits 20–80 and stroke range 0–100. The three feel runs lasted 160 s;
live editing 85 s; Slow build 95 s with a one-minute arc; Waves 185 s; Edge 280 s;
and Cooldown 95 s with a one-minute arc. Each retained stream has exactly one
Play and one final Stop, with no intervening restart or retarget blend. The
explicit Stop capture includes the queued tail; the renderer shades cancelled
points and excludes them from motion statistics.

| Feel, final steady capture | Stroke length p10 / p50 / p90 | Half-cycle seconds p10 / p50 / p90 | Continuations after start |
| --- | --- | --- | ---: |
| Gentle | 67.7 / 72.9 / 79.0 | 0.51 / 0.55 / 0.59 | 1 |
| Balanced | 57.2 / 67.3 / 88.5 | 0.35 / 0.40 / 0.53 | 2 |
| Intense | 33.8 / 45.2 / 59.1 | 0.23 / 0.29 / 0.34 | 5 |

The prior catalog-driven capture is retained as `claude-baseline.json` and
rendered alongside the final runs. Its repeated catalog blocks differ visibly
from the evolving stream. Its earlier helper did not retain explicit final
Stop, so it is character evidence, not the final Stop gate. Final live edits
preserve queued strokes and then ease into the new controls. Slow build rises
and holds, Waves swells and falls, Edge backs off into shorter tip work, and
Cooldown stops naturally about ten seconds after its one-minute arc.

These are commanded semantic samples and planned derivatives, not carriage
telemetry. Finite-difference acceleration of captured dispatch points is also
not an exact polynomial derivative. No physical device was connected or moved.
Real Cloud/Bluetooth/Intiface latency and felt acceptance remain open under R1.

## Application and validation

- Full `go test ./...`, Windows `go test -race -timeout 20m ./...`, `go vet`,
  `golangci-lint` and `CGO_ENABLED=0` build pass. The full HTTP API race package
  completed in 306.833 s locally; architecture and goleak gates remain enabled.
- All Freestyle mode tests pass 100 consecutive race-enabled runs after the
  forced mid-tick preference regression and completion-signal synchronization.
  The full mode race package, vet and lint also pass after those fixes.
- Typechecking, five-locale checks, all 757 frontend tests in 99 files and the
  canonical production UI build pass. No second shipping UI or stale bundle
  is introduced. Budgets are recorded in the scorecard.
- The isolated review app is built from this source with `-simulate-motion`.
  Its final configuration passes `scripts/check-review-llm.ps1` against local
  Ollama `huihui_ai/granite4.1-abliterated:3b`. The earlier Gemma endpoint
  disappeared during review, so the final app was reconfigured and verified
  again. Its text-only browser chat completes in 115 ms (first token 55 ms),
  with a non-empty reply and no repair, fallback or motion. The earlier Gemma
  probe (572 ms) is also retained locally.
- Browser checks cover a preset followed by a custom slider edit, starting
  Freestyle, changing the running shape from zero, and Emergency Stop returning
  it to idle. Desktop, 390-pixel and 320-pixel layouts keep the global Stop
  visible. Endpoint scale labels wrap within their available spacing at 320 pixels
  instead of overlapping the adjacent label.
  Handoff leaves Preset Modes idle, with Balanced and Steady selected.

## Reproduction

From the repository root, generate the shared-plan matrix:

```powershell
$env:MAGICHANDY_FREESTYLE_STREAM_DUMP = (Join-Path (Get-Location) '.scratch/freestyle-matrix.json')
go test ./internal/motion -run '^TestFreestyleStreamDump$' -count=1
python scripts/render-freestyle-streams.py .scratch/freestyle-matrix.json .scratch/freestyle-atlas
```

For real-time software captures, set `MAGICHANDY_FREESTYLE_CAPTURE` to an ignored
JSON path and run `go test ./internal/httpapi -run '^TestFreestyleSessionCapture$'
-count=1`. Its header documents duration, feel, shape and live-edit options.
Render with `python scripts/evaluate-freestyle.py capture.json output-dir label`.
Both renderers use the optional plotting dependencies already listed in
`scripts/requirements-motion-atlas.txt`; they are never shipped in the app.
