# Alpha.72 release review — 2026-10-24

Dates use America/New_York. The maintainer requested implementing the accepted
Cloud smoothness improvements and releasing an update, then requested Layered
as the new default and first mode, followed by Creative v2. PR #347 carries
the response, interruption, sampling and release changes against alpha.71.

## Source and acceptance

The [physical sampling review](cloud-sampling-review-2026-10-24.md) records
29 short Original Handy Cloud runs, all candidate representations, retained
failures, command plots and final production acceptance. D's fewer optional
position points were preferred over both more detailed and intermediate fits.
The even coarser E lost pattern definition. No turn-profile redesign is shipped.
The final slow and faster compound runs retained the improvement and stopped
correctly. Some turn hesitation remains. The misleading first “80%” comparison
and corrected full-group-rate repeat are documented explicitly.

The shared sampler applies a scale-aware approximation budget only to a single
Original Handy over Cloud. It retains the authored trajectory, clocks, mandatory
turns and safety bounds. Imported media, transition fitting, other profiles and
transports, and linked-device output remain unchanged. No new dependencies or
inference calls. The rejected velocity refinement is test-only. Budget evidence
is in the [scorecard](goal-scorecard.md).

The [response-depth review](model-response-depth-review-2026-10-23.md) records
106/106 final focused hosted checks, failed candidate outputs, non-motion intent
checks and local-model limitations. Hosted guidance distinguishes human travel
restrictions from model-selected coverage, and applied state from historical
failed requests. The stopped local Layered guide improves the focused sample
32/40 versus 31/40 without changing active local prompts. No deterministic
language classifier, forced speed band, or extra normal-success request is added.
The [chat investigation](chat-response-investigation-2026-10-23.md) covers the
interruption notice and honest timeout/reasoning diagnostics.

Layered is now the fresh-install default. Both product selectors order Layered,
Creative v2, Creative, Pattern Library and Off. Explicit saved modes, including
Off, and the older pre-selector migration are preserved. This does not authorize
motion at setup/startup. ADR 0015 records the product decision.

The exact review app uses an available local Gemma model and completes a direct
generation plus text-only app chat without repair/fallback or motion. Hardware
is stopped and disconnected at handoff. Claude Opus 5.5 Max returned no review
within the bounded consultation; no external approval is claimed. The installed
app and its data were not upgraded, restarted or reconfigured.

## Release gates and boundaries

Full Go and optional Labs suites, 967 frontend tests, typecheck/build, installer
tests, vet/lint, architecture checks and a pure-Go build pass locally. The Go
and frontend suites were repeated after the default/order change. Fresh-profile
verification confirms Layered; both visible selectors have the requested order.
The rebuilt final review app completed generation and a text-only chat without
repair/fallback/motion, then was left stopped in simulation with Layered selected.
Linux CI supplies race execution. PR and merged-main checks
must be green before creating the immutable alpha.72 tag at current main.

Only `0.1.0-alpha.72` is added to ADR 0014's reviewed unsigned policy. Alpha.65
and future alpha.73 remain rejected. No dependency, worker protocol, loader,
compression format, public artifact set or installer contract changes. The
existing source and five-executable vulnerability scans, Defender, provenance,
manifest/checksum verification and exact-installer lifecycle gates remain.
Microsoft's prior review does not approve the new hashes; trusted signing
remains the long-term target.

The tag workflow repeats validation and verifies the exact main commit before
publishing setup, portable ZIP and checksum file. Public notes disclose unsigned
status, update instructions and remaining Intiface Stop, physical multi-Handy,
loaded COYOTE and vibrator limitations. No physical claim is expanded beyond
the single Original Handy Cloud acceptance recorded above.
