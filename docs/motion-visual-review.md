# Visual motion review

Use this process when changing a motion recipe, generator, timing fit, sampler,
or model contract whose output needs assessment. Render the final motion that
the shared engine actually compiles. A graph of source control points alone
can hide timing distortion, additional reversals and quantization artifacts.

## Reproduce an atlas

The exporter is development tooling, compiled only with `magichandy_labs`. It
creates no transport and starts no playback goroutine. Run from the repository
root after building the development UI when package embedding requires it:

```powershell
go run -tags magichandy_labs ./cmd/motion-atlas `
  -output .scratch/motion-atlas.json -speeds 10,25,43 -legacy=true `
  -llm '.scratch/library-naming-v5.json,.scratch/llm-lab-interfaces-strict.json,.scratch/library-lab-live-final.json,.scratch/library-live-app-path.json'

python -m pip install --target .scratch/atlas-python -r scripts/requirements-motion-atlas.txt
$env:PYTHONPATH = Join-Path (Get-Location) '.scratch/atlas-python'
$env:MPLCONFIGDIR = Join-Path (Get-Location) '.scratch/atlas-matplotlib'
python scripts/render-motion-atlas.py .scratch/motion-atlas.json .scratch/motion-atlas `
  --captured .scratch/library-live-app-path.json
```

Omit `-llm` and `--captured` when those optional reports do not exist. Change
the speed list to the scenario being investigated; the current exporter uses
Original Handy limits 10–43 and rejects speeds outside that comparison range.
Engine tests separately cover all supported Handy models. Dependencies are
optional review tools only; no Python, plotting library or generated atlas is
embedded in either app build. Keep generated JSON/PNG/HTML and logs ignored.

Use `-catalog=false -experiments` for the guided continuous-flow experiments:
reference, drift, softness, cadence hold, combined, and maximum softness.
`-speeds 10,25,43` renders each at all three rates. LLM trials use captured
limits when available. The filter includes Flow experiments and opens on the
first available group. `motion_ideas_live_test.go` supplies tuning/continuation
reports; `MAGICHANDY_EXPERIMENT_CAPTURE` optionally exports the five-round
shared-engine/fake-transport test from `lab_experiments_test.go`.

The output contains `index.html`, `manifest.json`, distinct plot PNGs and
overview sheets. Every input has a manifest record. Identical sampled output
shares an image while each request, model, raw response and result keeps its
own card. Invalid responses and Stop have explicit records without a moving
plot. The page filters the new catalog, legacy catalog and LLM output, with
text search including failure labels. It can be opened locally or served from
the atlas directory on loopback; it has no app/device API access.

## What to examine

1. **Whole-loop position:** inspect both endpoints, range trends, resetting
   behavior, repetition and the seam. Confirm fixed-region, anchored-return,
   changing width, traveling center and pace variation are actually distinct.
2. **Planned versus quantized output:** the first 12 seconds overlay the shared
   plan and the buffered fitter's whole-percent points. Look for flattened
   reversals, extra pauses, chatter, large gaps and directional concentration.
3. **Phase portrait and velocity:** symmetric travel should have a balanced
   loop. Check spikes and concentrated travel in one direction rather than
   inferring smoothness from a rounded position plot alone.
4. **Acceleration and seams:** inspect exact polynomial acceleration and the
   reported largest acceleration jump across every knot, including the loop.
   Finite-segment jerk alone does not measure a discontinuity at a C1 seam.
5. **Requests versus selected output:** read the raw response and actual
   resulting geometry. A valid schema or plausible reply is not an intent pass.
   Include failures; do not render only successful selections.
6. **Startup, retarget and Stop:** steady-loop charts do not cover transitions.
   Add the app-to-captured-transport timeline and retain the raw commands/trace.
   Its points are actual semantic floats sent to the fake interface, separately
   labeled from quantized steady output. The timeline shades queued samples
   beyond the test's Stop time; those were canceled, not executed.

After inspecting all overview sheets, inspect detailed figures for every new
or changed recipe at representative low/middle/high speeds, plus outliers and
failed model selections. Record the artifact paths, parameters, findings,
resulting changes and limitations in the review document. Numerical kinematic,
wire fidelity and Stop tests remain required. For physical evaluation, record
the device, transport, limits, latency, trace and the user's actual feedback;
neither planned derivatives nor captured commands are carriage telemetry.

## Live report generators

`internal/chat/continuous_catalog_live_test.go` compares three naming variants
and validates parsed commands against compiled geometry. `llm_lab_live_test.go`
scores bounded controls/sections/layers, including preservation of every
unrequested field. `library_lab_live_test.go` tests selection and continuation
through the separate compact recipe contract. All use the opt-in `liveeval`
tag and `MAGICHANDY_LAB_MODELS` (installed model names separated by `|`).
`MAGICHANDY_LAB_REPORT` chooses the ignored JSON output.

`internal/httpapi/continuous_library_live_test.go` requires `liveeval` and
`MAGICHANDY_LIVE_MODEL`; its fixture explicitly enables Labs. It sends real model responses
through the HTTP chat path into a fake transport, verifies actual engine
targets, one-call generation, append/play continuity and Stop, then exports
commands, trace rows and steady plans for the atlas. No physical device is
used by any of these harnesses.


For live Lab session evaluation, run `scripts/evaluate-lab-conversation.py` with
`--base-url` pointing to a fresh isolated review app, one or more `--model`
arguments, and `--output` under `.scratch`. It takes controller ownership,
tests catalog naming and multi-turn edits, and runs preview-only Autopilot.
The report retains wrong choices as `expected_recipe`/`intent_pass`, alongside
raw output. The atlas reads these fields and labels actual compiled motion
separately from the expected selection. Capture the shared live-retarget path
with `MAGICHANDY_EXPERIMENT_CAPTURE` and
`TestLabConversationRetargetsOneSharedRun`; pass it to the renderer's
`--captured` option. See the September 5 evaluation document for an example.
