# Creative v2 roaming focus and example bias

The user clarified that the fast tip sweep was an example of a capability,
not a motion that should dominate autonomous sessions. The previous build
still repeatedly returned to an upper endpoint, sometimes sharply.

## Causes and retained user evidence

The bias was partly authored: `DefaultGestureSpec` used a 100% focus despite
being labeled neutral, variation kept that anchor fixed, and the first model
example demonstrated 65% directional contrast at the tip. A new seed could
change the other endpoint and timing without releasing the fixed endpoint.
Those defaults and examples could reinforce a model's own repeated choices.
There was no separate hardcoded command to hammer the tip forever.

The user's stopped Cloud REST trace was captured read-only in
`.scratch/creative-v2-region-review/user-*`. Its bounded export retains 128
rows and omits 352 earlier rows. Queued commands cover stream 287.615–389.875
seconds. Much of this tail occupies the upper range; the last five-second
window is approximately 69–99%. The 100 transport results have median latency
337.5 ms, empirical p95 377 ms and maximum 612 ms. The partial trace does not
contain the initial play timestamp or the complete semantic score history.
It establishes commanded geometry, not carriage feedback or the entire run.
The user's connection, current settings and motion session were preserved
during evaluation; no new physical motion was issued.

## General control change

Focus gains `roam_percent`, independent of stroke-width mixing. Zero preserves
an explicitly held location; 100 ignores the previous anchor and lets the
working location drift within the outer band. Intermediate values retain a
spatial preference without forcing one endpoint to remain stationary. Fresh
motion starts fully roaming, with no preferred end. Full-stroke-only requests
still span their requested band.

The shared gesture compiler generates a correlated location field. A periodic
rate bound keeps adjacent narrow windows overlapping, retaining alternating
reversals through the seam. This does not add a region itinerary, a time-based
rotation, a private sampler, runtime randomness or another motion path. Seeds
remain replayable; live evolution changes their realization. Existing timing,
velocity/acceleration/jerk limits, transport dispatch, handoff and Stop retain
authority.

Persisted scores without the new field use zero and retain their authored
anchors. Historical three-field focus transactions also retain fixed-location
meaning; current constrained LLM output supplies all four focus fields. This
preserves explicit saved motion rather than silently relocating it on update.

The prompt removes specific tip-sweep and rebound recipes. Examples illustrate
edit mechanics rather than preferred motion. Location and reach semantics are
stated separately, and Autopilot is reminded that a previous model-selected
anchor is not a human constraint. Active speech facts describe roaming instead
of claiming a fixed region. Lab help is updated in all five locales.

## Visual review and regression process

The matrix now includes three roaming combinations alongside the existing
anchored references and Original Creative comparisons: 135 cases, 112 distinct
figures and seven overview sheets. All seven sheets and each new combination
at representative low/middle/high speeds were inspected. Narrow roaming
strokes visibly move both endpoints; mixed reach combines that movement with
progressive width changes. Exact held references retain their fixed endpoint.
High-speed narrow work can still saturate the unchanged timing limits, and
the finite realization repeats without evolution.

The initial prompt candidate preserved `roam_percent:100` even when Gemma
named a specific local region. The earlier intent checker looked only at the
position field and missed one such error. The checker now requires held focus
for those spatial requests, and the failed reports remain retained. A second
candidate clarified roaming but still confused local-only and mixed reach,
and sometimes copied outer limits into relative placement. The final wording
separates the four controls and explains their coordinate systems. No named
motion recipe was reintroduced to teach that distinction.

A later available-space wording was rejected: Gemma's strict control result
fell from seven to six of eight. The accepted prompt retains the preceding
range definition. Independently, speech facts no longer claim that roaming
can relocate a stroke that already fills a minimum-width outer band. The new
capability harness explicitly supplies a broad outer range and checks that
room exists to move; a changed roaming parameter alone is insufficient.

Use `scripts/evaluate-app-controls.py` for existing motion contracts,
`scripts/evaluate-app-focus.py` for held → roaming → held transitions, and
`scripts/evaluate-app-autopilot.py` for no-request sessions. These require an
isolated simulator app. Reports include every response and authoritative
target, including failed selections, for the shared-engine atlas. All runtime
reports and visual artifacts remain ignored and unembedded.

## Accepted results and remaining weaknesses

Full Windows app tests used llama.cpp b9966 with Gemma 4 12B on CUDA and
Granite 4.1 3B on six CPU threads. These runs compare control behavior, not
model speed, because the execution hardware differs.

| Check | Gemma 4 12B | Granite 4.1 3B |
| --- | --- | --- |
| Baseline control requests | 8/8 valid, 7/8 strict intent | 8/8 valid, 7/8 strict intent |
| Accepted control requests | 8/8 valid, 7/8 strict intent | 8/8 valid, 6/8 strict intent |
| Held/roaming capability requests | 4/4 valid, 3/4 strict intent | 4/4 valid, 2/4 strict intent |
| Accepted-prompt Autopilot, 150 seconds without human motion requests | 11 accepted targets, 14 check-ins, Stop passed | 12 accepted targets, 10 check-ins, Stop passed |

Gemma preserves the baseline control score; the rejected additional range
wording scored 6/8. The final capability run successfully releases a held
anchor, holds the opposite end, then releases it again without changing other
controls. Its initial response incorrectly retains mixed reach for a request
for local strokes only. Granite also releases roaming successfully, but misses
the initial location/mix and later leaves roaming enabled instead of holding
the requested lower end. Its existing control suite loses one strict pass.
These are retained mapping defects, not repaired replies or claimed successes.

The combined live atlas retains 204 model/output records, 166 distinct steady
plots, eleven overview sheets and seven captured dispatch timelines. Every
overview and captured timeline was inspected, with detailed inspection of
held/roaming counterparts and failed spatial selections. The initial Gemma
held-upper output has a stationary upper endpoint; releasing roaming moves
both endpoints while retaining the reach field and pace. Holding the lower
end restores a stationary lower endpoint. Granite's failed hold request is
visibly still roaming. Sharp narrow work remains visible with strong travel
contrast; this change does not lower the device's existing timing limits.

Autopilot results named `v3` use the accepted motion prompt. Gemma selects
roaming amounts of 10–40 and still spends long stretches in broad repeated
strokes or a regional idea. Granite reduces roaming from 80 toward zero and
later 10–20 while moving its preferred placement upward. Its latter timeline
remains regionally repetitive. The four-minute Gemma capture named `final`
belongs to the rejected wording experiment and is retained as such; it is not
evidence for the accepted prompt. Neither live timeline proves that the
remaining flow or physical sharpness complaints are solved.

The engine no longer bakes the example endpoint into fresh motion, and it can
now move an existing working region without constructing a sequence. Models
can still choose a narrow outer band, lower roaming, select an end repeatedly,
or confuse local/full reach. A finite 64-cycle realization also repeats without
evolution. Further naturalness assessment requires physical feedback; these
tests use shared-engine commanded output through an isolated test transport.

Full Go tests, race tests, vet, lint and CGO-free builds pass. Frontend
typechecking, 473 tests, all five locale checks and the canonical production
build pass. New checks cover held-score compatibility, free-roaming anchor
independence, 144 narrow-window/replay fixtures, compiled motion combinations,
explicit roaming authorization, and truthful speech when no relocation space
exists. No dependencies were added. The stripped binary is 19,076,096 bytes,
4,608 bytes above the preceding flow build; current bundle, launch and memory
measurements are recorded in `docs/goal-scorecard.md` without closing the
existing memory waiver.
