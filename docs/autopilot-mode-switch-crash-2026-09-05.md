# Autopilot mode-switch crash, 2026-09-05

The real-mode Windows review process exited after a switch from Layered to
Creative v2 and an Autopilot restart. The retained app log ends immediately
after the 17:34:12 EDT start request with `panic: assignment to entry in nil
map`, at `creativeV2ScoreContext` in `internal/chat/creative_v2.go`. The stack
runs through Autopilot prompt construction on the mode manager's goroutine.
Both the app listener and its managed model endpoint were gone afterward.

The setting selects the next LLM control grammar before an accepted update
replaces the preceding motion score. A Layered score has no `Gesture` field.
The formatter chose Creative v2 from the selected setting, marshaled the nil
gesture as JSON `null`, unmarshaled that into its map, then assigned to the
now-nil map. This was a core process panic, independent of model reply quality
or Cloud REST latency. Earlier single-mode tests never supplied an active
score from the preceding grammar.

## Fix

Autopilot describes a retained continuous score using its actual schema. During
a mode handoff it distinguishes that active score from the selected mode's
authoritative edit state. The existing prompt/parser path still creates a valid
starting score for the selected mode. The Creative v2 formatter also returns
no context for a missing gesture instead of inventing controls or panicking.

Motion compilation, shared-engine ownership, sampler, sanitizer, kinematic
limits, transport dispatch and Stop behavior are unchanged. No catch-all panic
recovery or alternative motion path was introduced.

## Verification

- A transport-free regression reproduced the exact nil-map panic before the
  fix. It now passes for both score types and both selected continuous modes.
- Production API tests switch modes with a retained engine score, complete the
  next Autopilot decision using the selected grammar, preserve pace, apply it
  through the same engine and verify Stop. Both switch directions pass without
  a repair or fallback provider call.
- `scripts/evaluate-app-mode-switch.py` exercises a full Windows build against
  Gemma 4 12B through llama.cpp b9966. Four 25-second phases run Layered →
  Creative v2 → Layered → Creative v2 without clearing the previous score.
  Every phase reached a model-selected score of the expected type, the app
  stayed available, and final Stop passed.
- The capture contains ten observed targets (including the retained target at
  each switch), seven distinct compiled outputs and three rejected geometry
  decisions. The rejected decisions held the preceding motion and remain in
  the report. The atlas renders all thirteen records, seven plots, one overview
  and the dispatch timeline. The overview, both switch-direction outputs and
  timeline were inspected. One transport play spans the entire run, with 103
  captured transport commands and the final canceled queue shown. This is
  commanded simulator output, not a physical feel or device-latency test.
- Full Go tests, race tests, vet, lint and CGO-free builds pass locally. The
  canonical frontend is unchanged; PR and tag CI rerun frontend, installer,
  package and release gates. The stripped local build remains 19,076,096 bytes.

Runtime logs, model output and rendered evidence are retained only under the
ignored `.scratch/autopilot-mode-crash` directory. The original crash log is
retained under `.scratch/creative-v2-region-review`. The review is restarted
in real-device mode with the existing saved settings. Its current conversation
was checked before startup reconciliation, and saved conversations were kept.
No new physical motion was issued during this investigation.

The update is included in alpha.40, with the unchanged explicit artifact policy
and full tag-pipeline scan, verification and installer lifecycle requirements.
