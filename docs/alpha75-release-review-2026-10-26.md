# Alpha.75 release review — 2026-10-26

Dates use America/New_York. The maintainer authorized implementation of the
physically accepted Layered timing candidate and publication, including the
pending voice, Graphite UI, Stop/network and Autopilot reliability work on
`codex/voice-reliability`. The base is `ad5450c2`, after alpha.74. User-installed
settings and credentials are not copied into review profiles.

## Scope and evidence

- [Voice review](voice-reliability-review-2026-10-26.md): coalesced managed
  loading, health recovery, bounded cancellation, audio-stream failure
  reporting/retry, pressure simulations and live Qwen/Parakeet checks. Claude's
  Graphite settings implementation and subsequent fixes are included.
- [Stop/network review](stop-network-error-review-2026-10-25.md): explicit
  cancellation reasons, bounded stream drain, keep-alive recovery and notices
  anchored to the interrupted request.
- [Autopilot review](autopilot-admission-review-2026-10-26.md): trusted generated
  prompt admission, failure visibility/recovery and actual-provider provenance.
  It retains the failed GPT-6.1 attempts alongside successful GPT-6 Sol trials.
- [Layered speed review](layered-speed-support-review-2026-10-26.md): physical
  full, short and counted-pattern A/Bs over Original Handy Cloud. The user
  judged the candidate faster and smooth in all three, preserving compound
  definition; full-range Stops were physically confirmed. Commanded mean
  increases were 38%, 90% and 54%, not carriage-velocity measurements.

## Production motion integration

Layered's clock expresses requested rate, authored pace/rate modulation and
cadence hold. The shared exact joint fitter supplies kinematic limits, avoiding
the former conservative clock followed by a second fit. Layered now shares
Creative v2's 7500 percent/s² and 150000 percent/s³ envelope. Device velocity,
user bounds, Stop, transport mapping and the sparse Cloud sampling policy remain
in force. Neutral constrained strokes can use the existing cruise candidates;
unconstrained and explicitly softened turns retain their intended shape.

Production fixtures reproduce the physically accepted six-cycle periods:
4116 ms full, 1536 ms short and 2660 ms counted. Numerical extrema refinement
could otherwise add a millisecond to alternating equal legs and seed unequal
joint fits; negligible relative search error is removed before rounding.

Integration tests exposed a slow-to-fast handoff with no feasible single
quintic to the fast destination reversal. An interior join to the same stroke
now matches position, velocity and acceleration without skipping its turn or
count. Exact monotonicity, peak velocity, acceleration, jerk and a single
velocity crest gate acceptance. A finer bounded duration search handles narrow
feasible intervals. The sampled continuity test derives its two-millisecond
allowance from the actual jerk envelope; exact C2 checks remain at 1e-7.

A separate fast-transition sampler failure was caused by using a distant
sparse probe gap to reserve interpolation error for a densely probed segment.
Final timing verification now reserves A*dt²/8 for the probes bracketing that
segment. It does not enlarge the configured position-error budget or relax
velocity/timestamp/reversal checks.

The UI displays backend effective/requested pace for Layered and Creative v2,
labels it an estimate and explains that it is not measured device speed.

## Validation and release status

Local full Go and race suites, vet, lint (zero issues), frontend typecheck,
five-language audit and all 1064 frontend tests pass. Seven Python voice-adapter
tests pass in an isolated environment with the repository's test requirements;
the system interpreter initially lacked FastAPI, so it was not used as a
passing result. Windows PowerShell 5.1 installer checks and the release-policy
architecture tests pass. The source vulnerability scan reports no reachable
or imported-package vulnerabilities; one unused required-module advisory
remains reported. The production motion atlas covers
132 combinations of device profile, speed, reach, sections, compound rates,
softness, cadence and layered modulation; its runtime capture includes a
slow-to-fast retarget and Stop with simulated Cloud latency. All eight overview
sheets, detailed outliers and the runtime capture were inspected. Recorded
Autopilot outputs were also replayed through production, retaining provider
failures; this does not claim new live model trials. The current review app
completes real local generation and a text-only chat with no repair, fallback
or motion. The final text-only request completed in 113 ms with one provider
call. The simulator showed the full requested-100 curve at 79% estimated
effective pace. A visual pass caught clipped values in the narrow sidebar;
aligned label/value rows now keep range, both pace values and source legible.
The review app at `http://127.0.0.1:50303/#/chat` remains isolated, simulated and
stopped, with LLM motion off. UI captures are retained with the local evidence.

The bounded optional curve search raises compile cost at high selected speeds:
the three tested six-cycle fixtures take 0.166–1.118 ms median on this host;
the slow fixtures remain 0.048–0.135 ms. This is not model latency and adds no
inference request. Detailed before/after allocations are in the scorecard.
Final package and CI provenance belongs to the PR and release workflow, since
it cannot be recorded in the exact source commit after that commit is tagged.

No production dependency is added. Detailed budgets are recorded in the
[scorecard](goal-scorecard.md). All runtime reports, raw captures, plots and
test binaries remain ignored under `.scratch/alpha75/` and the linked review
directories. Keys are excluded from evidence and release files.

Only alpha.75 is added to the reviewed unsigned release policy. Source and
binary vulnerability scans, Defender scanning, manifests/checksums, full
installer lifecycle, green PR and merged-main CI, and exact tagged-main
provenance remain mandatory. There is no signing claim or gate bypass.

## Remaining limits

The installed alpha.74 full-range run reportedly felt slower than the matched
baseline despite similar captured command timing. No failed commands or
obvious buffer starvation were found; that discrepancy remains unresolved.
Physical acceptance is Original Handy/Cloud and the three documented patterns,
not every modulation, transport or Handy model. The voice reporter's exact
4070 Ti/model/reference combination and extended listening remain unverified.
The requested Claude Opus 5.5 xhigh motion review exhausted its session limit
before producing findings and is not represented as a completed sign-off.
