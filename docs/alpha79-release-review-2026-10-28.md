# Alpha.79 integration review — 2026-10-28

## Source and release scope

The maintainer explicitly requested an update and delegated publication to
Updates. Source is `codex/layered-session-repair` at `739f36a5`, based on alpha.78
main `69a33dc0`, in the existing `MagicHandy-vibrator-level` checkout. Remote
discovery confirmed alpha.78 was latest and alpha.79 was unused. No unrelated
checkout is reset or switched. Release preparation adds notes and the
version-specific policy to the same PR; all source must merge through reviewed
green CI before tagging the exact current main commit.

The [session repair review](layered-session-repair-review-2026-10-28.md) and
[ADR 0052](decisions/0052-counted-layered-phrases.md) contain the diagnosis,
failed candidates, model trials, detailed checks and physical test plan.
Edge no longer seeds hidden 85% turn softness; production Creative v2 no longer
inserts seeded 110–280 ms rests. The separate Lab stroke vocabulary remains.
Phrase+recall, redundant absent-layer removal and resolved-value diagnostics
are corrected. Advanced gains complete local regional cycles with connecting
travel and explicit direction-rate multipliers through the same engine.

## Safety and evaluation boundaries

Regional bounds/counts are validated atomically; small contradictory reversals
are rejected. Same-direction waypoints merge with distance-weighted travel
timing, preserving equal rates exactly. Retained semantic slots guard ordinal
handoffs. Unequal direction rates use one fitted motif clock, including the seam,
to preserve relative durations inside the existing velocity/acceleration/jerk
envelope. Slow companions stretched to preserve rhythm keep rounded shaping.
Symmetric/default timing, shared sampler, sanitizer, transport protocol, saved
windows, hardware limits and Stop/recovery handling remain unchanged.

The new regional and direction vocabulary is Advanced-only. Direction edits
from unadvertised tiers are rejected; tier switches preserve an already playing
asymmetric score as read-only context. Final Terra Advanced regional and
direction suites each passed 27/27 first-pass intent checks; the production
one-repair path passed 2/2. Simple E4B 17/24 and Intermediate 12B 21/24 candidates
were not enabled. Follow-up guidance and the original failures remain retained.
CPU-only local worker timings are not GPU benchmarks. Three Claude reviews
supplied findings that were addressed; they do not approve subsequently revised
code. There is no language classifier, preferred speed/ratio or hidden floor.

Retained visual evidence covers 502 motion records/287 distinct plots in 18
overview sheets and 270 model trial records/30 distinct plots in two sheets,
plus detailed outliers and the buffered runtime timeline. It includes failed
selections, rejected compiler candidates and explicit admission errors.
Whole-percent wire overlays remain commanded estimates, not carriage telemetry.
The shared fitter may lower average speed to preserve directional contrast;
slow asymmetric endpoint portions require physical review.

The source thread completed 486 profile/speed/reach/grammar/direction checks,
120 seeded regional cases, exact C2 seams/envelope tests, symmetric timing,
semantic slot/count checks, and an inert 350 ms append-latency retarget/Stop
scenario with one stream/Play. Full ordinary and race Go suites, vet, normal
lint (zero issues), architecture, pure-Go build, frontend typecheck/build and
1,069 tests passed. The earlier concurrent frontend/race asset collision remains
recorded; the full race suite passed after canonical assets stopped changing.
Updates independently reviews source and retained plots, reruns focused safety
and release-policy checks, and requires final PR, main/package and tag gates.

## Budgets and artifact policy

The [scorecard](goal-scorecard.md) records stripped review core 24,376,832 B
(+39,424 B from alpha.78) and canonical UI 2,856,207 B (-202 B). Shipping retains
symbols with `-w` and no `-s`. No production dependency, worker, timer, inference
call or managed runtime pin is added. No RSS/cold-start improvement is claimed.

Only alpha.79 joins ReviewedUnsignedPublic. Alpha.65 remains excluded and
alpha.80 remains rejected. Trusted signing is unavailable; Microsoft's old
case does not approve new hashes. Defender of the exact public directory,
source and all five exact-binary scans, native x64/non-solid setup, complete
installer lifecycle, manifests/checksums, three explicit public assets, clean
source and current-main tag provenance remain mandatory. Updates audits the
actual published downloads before handoff.

## Installed session and review app

The installed app is actively in use. Its settings, controller, credentials,
browser tab, model allocation and playback must not be changed. No real device
is connected or commanded. Real hardware is Original Handy Cloud; the temporary
Handy 2 Standard setting was a speed workaround, not a hardware change.
The two captured Cloud recovery stops remain unexplained. Recovery is not
suppressed, and this alpha does not claim all real-world stutter is eliminated.
Physical acceptance remains open, alongside earlier vibrator and Intiface Stop
limits. Follow the source review's scenarios only after explicit authorization.

Preserve the current-source isolated app at `http://127.0.0.1:50309/#/chat` and
its CPU-only Gemma 12B worker at port 50311. It uses simulated devices, remote
listener disabled, no copied credentials, Intermediate/32K and LLM motion Off.
The source thread's real-generation probe and text-only app request passed
(5,034 ms, one call, no repair/fallback/motion). Updates revalidates the exact
app, leaves its relevant tab open with Motion Idle and Autopilot Off, and keeps
all other user/review processes running.
