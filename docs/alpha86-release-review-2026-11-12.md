# Alpha.86 release review — 2026-11-12

## Accepted source and scope

The reviewed `claude/vacuglide-native` working tree was prepared on
`codex/release-alpha86`, based on alpha.85's merged main commit `f4e67ef6`.
The maintainer requested publication after completion of the native-device,
Graphite control UI and Autopilot timing review. The implementation, failed
experiments, corrected timing evaluation and hardware limits are recorded in
[the completion review](vacuglide-completion-review-2026-11-12.md),
[ADR 0071](decisions/0071-vacuglide-native-tempo.md) and
[ADR 0072](decisions/0072-autopilot-timing-without-switches.md).

This release adds shared-engine native tempo for VacuGlide, bounded optional
air adjustments, tiered model contracts, seeded routines and continuation,
native Edge, the revised Now playing/Play interface, and simplified Autopilot
timing. Session buildup remains an optional finish target. Request-age context,
timing vocabulary, stale buildup decisions and UI participation reporting were
corrected during review. No deterministic language classifier was added.

## Validation before publication

The final implementation passed full Go normal/race suites, vet, lint,
architecture and goroutine-lifecycle checks, the pure-Go build, frontend
typecheck/localization/build, and 1,123 frontend tests. Production-prompt native
regressions passed 30/30 Terra, 29/29 Gemma 12B and 23/23 E4B cases, plus the
final explanatory-copy checks. All evaluated and rejected results were retained;
the completion review distinguishes timing misses from contract successes.

The reviewed app at `http://127.0.0.1:50363/#/chat` uses isolated simulated
devices and working local Gemma 12B. A real text-only chat response and the
readiness probe passed without repair, fallback or physical motion. The
user-installed sessions and the signed-in ChatGPT profile on 50360 remain
untouched. Budgets are recorded in [the scorecard](goal-scorecard.md).

Release preparation extends the explicit unsigned policy to alpha.86 only,
with alpha.87 and the excluded alpha.65 still rejected. It changes no payload
format, dependency, runtime pin, scanner, installer lifecycle or CI gate.
PR packaging and tests, merged-main checks and the tag-triggered release checks
must pass before publication is reported. The public downloads must then be
checked against the published checksums and source manifest.

## Limits carried into the release

VacuGlide remains experimental. No real device was connected or commanded in
this review. Motor range/stall floor, cloud latency, valve-step effects, Stop
during valve activity, reconnect and mixed-device physical behavior still need
hardware acceptance. Mixed runs follow a position lead; native tempo runs leave
incompatible selected companions idle. Independent mixed-device tempo prompting
is not included. The accepted cloud-only Stop limitation remains explicit.

Timed instructions are model-dependent. Small local models missed some timing
scenarios, and Layered developed pace at reconsiderations rather than meeting
an exact deadline. Neither a buildup bar nor verbal timing guarantees a specific
physical speed, a 100% finish, or an automatic Stop.
