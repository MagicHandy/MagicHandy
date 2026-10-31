# Alpha.82 release integration review — 2026-10-31

## Scope and authorization

The owner requested an update and handoff to Updates after the focused cadence
comparison. Release source is `codex/session-pace-recovery`, authored commits
`20ff0a70`, `93468e13`, `18fa0b9a` and docs-only `2c2fc98d`, based on released
alpha.81 `a519fe7eb7bf9bc0e0820550719455a43ff12742`. The installed app and
unrelated checkout are preserved. Updates owns PR, review, green CI, merge,
merged-main checks, immutable exact-main tagging and published-download audit.

See the [source review](session-pace-recovery-2026-10-31.md) for the full
investigation, comparison parameters, numerical checks and acceptance record.

## Behavior and safety

Directional fields retain local constrained timing with neighboring slow legs
stretched enough to preserve direction contrast. Distant unconstrained legs
avoid the whole-loop penalty. Counted phrases use direct common-clock fitting;
symmetric fields retain the prior path. After sixteen local passes, the bounded
common-clock solver receives the original requested timings. Invalid curves
remain rejected. Endpoints, counts, kinematic limits, sparse sampling, shared
engine/transport ownership and Stop remain authoritative.

Typed failed arrival halts autonomous retries before asynchronous teardown,
including lifecycle-lock contention. A new explicit Start may retry with the
same startup verification. No language rules, prompts, dependencies, workers
or additional motion path are introduced.

The owner selected reset and recheck only. The backend recovery snapshot
survives idle and carries an opaque failure ID. Controller admission rejects
stale/duplicate actions and Stop-during-decode races. The engine start barrier
excludes new motion; Stop cancels reads before waiting on the command lock.
Engine replacement invalidates admission or cancels the old read, while device
I/O does not hold the server lifecycle lock. Confirmed Stop plus fresh valid,
stationary position/calibration readings are required for each participating
slider. Success remains stopped and grants no arrival exemption. UI state and
dismissal follow backend snapshots; notices distinguish failure categories and
reset-specific cancellation without exposing credentials or raw provider prose.

## Review and validation evidence

Claude Opus 5.5 xhigh supplied a read-only second opinion before the final
follow-up. It found no critical/high issue. Reproduced low-severity admission,
lifecycle and wording findings were fixed. This is not a claim that Claude
approved every final line. Its medium cadence concern was evaluated physically
as described below.

The source's final production validation passed full Go tests and race tests,
vet, zero-issue lint, architecture/goroutine checks, pure-Go production build,
frontend typecheck/localization/build and all 1,090 frontend tests. Reset tests
cover invalid telemetry/geometry, moving sliders, failed Stop, linked devices,
stale/duplicate IDs, cancellation, engine replacement, request-decode races,
new-start exclusion and subsequent verified starts. A 108-case recompile after
review fixes changed zero trajectories and produced zero failures. The retained
atlas contains all 699 evaluated outputs, 26 overview sheets and detailed
outliers, including the insufficient direct-clock alternative. All eight
focused physical fixtures were also rendered through the shared compiler.

Updates independently reviews source and retained visual/evaluation evidence,
reruns focused safety/release checks and requires all final PR/main/tag gates.
Runtime evidence and private captures remain ignored under `.scratch/`.

Updates reproduced one additional classification gap in six regression cases:
invalid position, speed or calibration telemetry during arrival or stopped
verification returned an untyped error, leaving autonomous retry enabled and
omitting the recovery notice. Arrival validation now wraps that failure with
`ErrStartupArrival` and the invalid-readings category. The tests require stopped
state, the invalid-telemetry notice, one lead-in Play only and a final Stop.
Transport/read errors retain their existing handling; no bound or trajectory
changes. This release correction follows the earlier Claude review.

## Focused physical acceptance and remaining limits

Original Handy, Cloud, saved window 0–97%, eight-leg short-reversal/long-transfer
fixture, eighteen seconds per run: four speed/direction conditions (78/100%,
85/100 and 100/85 rates), first pair repeated, ten runs across five pairs.
The owner preferred retained local fit A and described it as natural in all
four conditions. Every startup arrival passed; 446 running snapshots stayed
running without engine error, all 170 uploads succeeded, and Cloud confirmed
stopped after every run and final check. Upload median/p95/max was
336/376/541 ms; end-of-run Stop median was 352 ms, range 327–1,681 ms. No
mid-run restart occurred. The owner confirmed clean Stops in the first/final
pairs. Keep A; this audition warranted no new production change.

The release request follows this focused acceptance. It does not assert that
the broader captured-field/live-retarget matrix is complete. That matrix and
wider vibrator/COYOTE/Intiface acceptance remain documented follow-up. The
unblinded, single-device comparison did not reproduce or explain the earlier
startup mismatch. HTTP 200 with app `ok:false` is not proof of literal Cloud
command rejection; the initial timeout and later response-processing failures
remain distinct evidence. Reset acknowledges fresh stationary readings and
does not claim to repair calibration or a root Cloud outage. Arrival tolerance,
fail-stop and subsequent start verification are retained.

## Budget, artifact policy and review app

The [scorecard](goal-scorecard.md) records stripped Win amd64 core plus UI
24,478,720 B (+44,032 B versus alpha.81), main JS +3,517 B (+851 B gzip), and
main CSS +406 B (+59 B gzip). No RSS/cold-start improvement is claimed. Shipping
builds retain symbols for exact-binary scans; the llama.cpp pin is unchanged.

Only alpha.82 joins ReviewedUnsignedPublic; alpha.65 remains excluded and
alpha.83 remains rejected. The old Microsoft case does not approve these
hashes. Exact-artifact Defender, source/five-binary vulnerability scans, native
x64/non-solid setup, full installer lifecycle, manifests/checksums, three public
assets, clean source and exact-current-main provenance remain mandatory.

Preserve the user-launched simulator at `http://127.0.0.1:50338/#/chat` and
stopped physical audition app on port 50339. The original simulator uses the
handed-off production source, a stationary fixture and local Gemma 12B Intermediate/32K
through Ollama. The source's final text-only reply completed in 391 ms without
repair/fallback/motion. Updates builds a parallel isolated simulator for the
arrival-classification correction, checks real generation and the app chat path,
and leaves its review tab open, Motion Idle, LLM motion Off and Autopilot Off.
No further hardware test or installed-data recovery is authorized here.
