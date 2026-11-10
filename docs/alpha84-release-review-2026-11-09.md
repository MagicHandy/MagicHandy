# Alpha.84 release integration review — 2026-11-09

## Accepted source and scope

The maintainer requested review of Claude's latest changes and another release.
Updates prepares release policy independently while the source reviewer owns
the source fixes and validation. The base is published alpha.83,
`4cc745d0dd44d147a66630635ad8009a6cfdc1fd`. Preparation uses
`codex/updates-alpha84`; source review uses `codex/release-alpha84`.

The completed handoff is PR #366, exact source commit
`d3dbb4b7f32ef32b3d32205f793e396c29aee9f3`. Claude's original uncommitted
snapshot was byte-verified and its worktree left untouched. The handoff
transfers integration and publication ownership to Updates, with all required
review and CI gates intact.

Advanced live Layered and Creative v2 chat receives guidance to keep replies
and motion consistent across invitations, feedback and rests. Source review
found that complete rests were described although the live continuous contract
could not emit Stop. The correction adds an Advanced-live-only bare Stop,
with empty edits and no simultaneous vibration change. It uses the existing
shared Stop dispatch, ends Autopilot, and is valid while paused without
resuming playback. A later fresh request can start through the same engine.
Simple/Intermediate prompts and autonomous action contracts retain their
existing behavior. No new language matcher is introduced.

The evaluation harness now fails and retains generation, compilation and
judge errors, excludes rejected assistant history, validates judge fields,
and avoids plotting remembered motion after Stop. An older lifecycle checker
accepts both supported counted-phrase formats. No frontend, engine, sampler,
fitter, transport, limit, dependency or runtime pin changes. See
[ADR 0067](decisions/0067-advanced-chat-keeps-scene-reply-and-motion-in-step.md)
and the [source review](roleplay-flow-review-2026-11-09.md).

## Release policy

Only `0.1.0-alpha.84` is added to ReviewedUnsignedPublic. Alpha.65 remains
excluded and the future-version rejection test advances to alpha.85. The
active versioning/packaging documentation and ADR 0014 move with this proposed
version while earlier release amendments remain historical records.

The native x64 Inno Setup loader, non-solid ZIP, five-executable payload,
three public assets and installer contract remain. No CI workflow, compiler,
scanner, dependency or managed-runtime pin changes. Source and exact-binary
vulnerability scans, exact-public-artifact Defender, full installer lifecycle,
manifests/checksums, green CI and exact-current-main tag provenance remain
mandatory. The previous Microsoft determination does not approve new hashes.

## Validation and budget

Preparation checks cover the release-policy change on the alpha.83 base. They
do not validate the pending source or replace any final-head check:

- Full installer suites passed on Windows PowerShell 5.1 and PowerShell 7.
- Architecture checks passed.
- Source vulnerability preflight reports zero reachable vulnerabilities and
  three module-only advisories in code not called by the app.
- Diff whitespace checks passed; the final committed preparation is checked
  for tracked-content integrity before handoff.

The initial parallel installer runs contended for the same startup-fixture
port, 11600. Windows PowerShell 5.1 passed; the PowerShell 7 failure log was
retained and its suite passed when run sequentially. No source or assertion was
changed to bypass that collision. Further installer runs use one suite at a
time and never stop another session to free a port.

Source review passed the full Go suite, vet, lint, pure-Go build, harness unit
checks and focused Stop/late-response race tests. Fresh GPT-6.1 Terra tests
passed 27/27 lifecycle cases and two five-turn feedback conversations without
repair or fallback. They cover invitations, keep-unchanged, model-selected
Stop, post-Stop reactions, paused/disabled states and a later fresh start.
These are focused observations, not a guarantee of model-selected pace.

All 594 retained final Claude proposals were recompiled through the shared
engine into 262 distinct plots and 17 overview sheets, inspected in source
review with detailed outliers 4, 28, 198 and 493. All 37 fresh turns compile
into a separate nine-plot atlas. Failed selections and provider failures remain
retained. The larger judge comparisons predate the explicit Stop correction;
they are not presented as a new benchmark of that addition. Updates checks
the Stop diff, retained captures and selected visual outliers independently
before recording its final-head review.

Limits remain explicit: a selected 1% slowdown is nearly stationary; another
100% base-speed proposal averages only 7.5% travel per second because of its
timing. Captured medians rose 2967 to 3694/3654 ms for Terra and 1076 to
1883 ms for DeepSeek. These are uncontrolled end-to-end trials, with no new
inference round on ordinary successful requests. Autonomous planning between
chat turns, non-English prompts, compact-tier models and physical feel were
not revalidated. The single judge and remaining pace-selection errors limit
the claims. Commanded estimates are not carriage telemetry. No hardware was
connected or commanded in this release review.

The same-host alpha.83 baseline was rebuilt using Windows amd64 Go 1.26.10,
`CGO_ENABLED=0 -trimpath -buildvcs=false -ldflags '-s -w'`: **24,490,496 B**
including the canonical UI. The UI is **2,885,533 B** across nine files; main
JS is **1,323,641 B**, gzip **355,646 B**, and CSS is **210,443 B**, gzip
**36,663 B**. These are baseline measurements, not a final candidate result.

Updates rebuilt the integrated source with the same flags: **24,495,104 B**,
an increase of **4,608 B**. All canonical UI size and gzip measurements are
unchanged. Shipping builds retain symbols for exact vulnerability scans.
No idle RSS or cold-start improvement is claimed. The scorecard and public
notes record the accepted behavior and limits.

## Review app and final gates

The exact-source simulator at `http://127.0.0.1:50358/#/chat` uses fresh
isolated data and local Gemma 12B. Source review completed real generation
and a non-empty text-only app chat in 309 ms without repair/fallback or motion,
and left its tab idle. Updates confirmed another real generation and reuses
this process because integrated production source matches the handoff. Final
visible-state and real-generation checks are required before handoff. Other
running user and Claude sessions remain preserved. The old unstarted alpha.83
baseline request on port 50355 is not needed for this current app.

Updates requires all final-head PR checks and review, merge by PR, green
merged-main checks, and an unused immutable alpha.84 tag on that exact main
commit. Tag/publication gates and independent verification of all actual
public downloads remain mandatory. The source PR is superseded only after
its exact commit reaches main.

No hardware connection, motion command, installed-session restart or session
replacement is part of preparation. Runtime, data, logs, captures and build
helpers remain ignored under `.scratch/`.
