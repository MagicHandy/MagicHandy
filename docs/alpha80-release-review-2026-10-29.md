# Alpha.80 integration review — 2026-10-29

## Scope and source

The maintainer requested publication after the independent Claude regression
review. Source is `codex/openrouter-reasoning` at `0aef6c1d`, based on alpha.79
main `f21853ab`. Four application/evidence commits cover the reviewed top-speed
path, its accepted Original Handy Cloud comparisons, OpenRouter reasoning and
Advanced Layered vocabulary, and the reproduced regression fixes. The separate
vibrator/COYOTE follow-profile PR #358 is not included. No unrelated checkout
is reset, switched or merged into this release.

The [motion-path review](motion-path-review-2026-10-29.md),
[Cloud comparisons](top-speed-cloud-audition-2026-10-29.md),
[model evaluation and Claude review](openrouter-reasoning-review-2026-10-29.md),
and ADRs [0064](decisions/0064-top-speed-reversals.md) and
[0039](decisions/0039-hosted-model-connections.md) contain the detailed evidence.
The release keeps the one semantic motion path and existing Stop/transport
boundaries. It adds no model, dependency, worker or automatic provider retry.

## Review and validation

The motion review retained 4,125 comparison records, 19 overview sheets,
detailed outliers, rejected filter removals, 315 buffered BLE cases and 244
supplemental release/session cases. Removing older cleanup was tested rather
than assumed beneficial: changes that introduced tiny reversals or exceeded
wire-error budgets were rejected. Eight real Original Handy Cloud runs at
100% completed stopped. The owner accepted the faster candidate for full,
short and two-short/one-full patterns without added harshness or lost rhythm.
Commanded timing and actual feedback remain distinguished from motor telemetry.

Claude Opus 5.5 xhigh independently reviewed the provider/vocabulary work.
Reproductions confirmed the readiness-versus-chat token mismatch, inconsistent
Autopilot/reviewer width names, obsolete repair field names, lost reasoning
error context, and selector regressions. Accepted corrections preserve semantic
values, rejection atomicity and typed causes. Simple/Intermediate prompt
contracts and ChatGPT/OpenAI request budgeting remain unchanged. The follow-up
does not claim new live-model quality measurements for local Advanced.

On the application source, full Go tests, full race suite, vet, lint (zero
issues), import/lifecycle checks, pure-Go build, frontend typecheck/build,
five-locale audit and 1,076 UI tests passed. Fresh targeted race tests passed
after the final test cleanup. Release preparation separately exercises the
version policy and installer scripts. PR, merged-main, packaging and tagged
release workflows must all pass before publication is considered complete.

## Budget and release policy

The stripped review core is 24,414,208 B, versus 24,376,832 B in alpha.79;
canonical UI is 2,862,704 B versus 2,856,207 B. The scorecard records the
individual motion/LLM changes and compile-time costs. Shipping retains symbols
with `-w`, without `-s`, for exact-binary vulnerability analysis. No managed
llama.cpp pin or install payload composition changes.

Only alpha.80 is added to ReviewedUnsignedPublic. Alpha.65 stays excluded,
and alpha.81 remains rejected. This is a version-specific review decision,
not a general relaxation: source and all five payload binary scans, Defender
on the exact public directory, native x64/non-solid setup, full default/custom
installer lifecycle, clean-source provenance, manifests/checksums and three
explicit public assets remain required. The old Microsoft case does not
approve new hashes. Trusted signing remains the target.

Tag `v0.1.0-alpha.80` must point to the exact current main commit after green
PR review, merged-main tests and a main packaging run. The immutable tag's
workflow repeats release gates before publication. Public downloads must be
audited for the expected three assets, hashes, version and source commit.

## Review app and remaining limits

The isolated current-source app is at
`http://127.0.0.1:50319/#/settings/chat/model`, with simulated devices, the
remote listener disabled, working local Ollama Gemma 12B at Intermediate/32K,
LLM motion Off and Autopilot Off. Its generation readiness probe and actual
text-only app request passed without repair/fallback or motion. Review builds
contain no copied cloud credentials. Preserve existing installed/review sessions.

The accepted physical tests do not cover every speed, asymmetric return,
Edge/live-retarget transition, Handy 2, other transports or multi-device case.
The two earlier Cloud recovery stops, real vibrator acceptance and documented
Intiface Stop limitation remain open. This release process connects no hardware
and makes no blanket claim that all physical stutter is resolved.
