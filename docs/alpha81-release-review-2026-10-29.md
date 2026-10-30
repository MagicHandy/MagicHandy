# Alpha.81 independent release review — 2026-10-29

## Scope and disposition

Reviewed Claude's uncommitted `claude/deepseek-parity` work against alpha.80
`1f89a3b9e9bb50b6685bd89b7c1fb11da03daca0`, preserving its source worktree and
capturing the patch in ignored local evidence. The release branch is
`codex/release-alpha81`. At the owner's explicit request, the release also
includes PR #358 (speed following and COYOTE); its independent follow-up is
recorded below. No dependency or llama.cpp runtime change is part of this release.

Retain the optional Advanced (no reasoning) capability, connection-specific
output floors, effort-sized OpenRouter reasoning reserves and host order UI.
See [ADR 0065](decisions/0065-advanced-no-reasoning-and-openrouter-limits.md).
The independent review also made these corrections:

1. Reproduced repair UI hidden for the new Advanced tier (one failing test),
   then fixed both repair visibility and the misleading second-opinion offer.
2. Clarified the capability hint in all five locales: the note applies to chat
   motion and selecting the tier does not switch reasoning off for the provider.
3. Distinguished temporary provider rate limits from known quota exhaustion,
   without retaining arbitrary provider error text or adding automatic retries.
4. Removed raw error-prose capture from the opt-in evaluation tracer. Only safe
   numeric error codes are recorded, including HTTP-200 streamed errors.
5. Added new-tier resume coverage and a live production-contract test that uses
   the real capability path and the actual 1,024-token answer floor.
6. Clarified full-device travel versus a human-restricted band in the opt-in
   prompt, after a live model kept the current 5–95 working band for full travel.
   No deterministic language rules or validation bypasses were added.
7. Corrected the evaluation report: Go-subtest completion was not consistently
   equivalent to intent success, and the Terra baselines combined partial runs.

## Model evidence

The [corrected review](openrouter-deepseek-parity-review-2026-10-29.md) retains
actual intent outcomes and replication limits. Its historical production-turn
checks are 318/343 and 315/343 for the composite Terra baselines, 270/343 and
280/343 for DeepSeek Advanced, and 302/343 and 297/343 for two note revisions.
These are turn-weighted automatic checks, not independent conversations or
human quality ratings. Compound and repair weaknesses remain documented.

Final matched eleven-case production-path trials passed for all three choices:
DeepSeek Advanced (739 ms median), DeepSeek with the note (1,048 ms), and Terra
Advanced/low reasoning (4,962 ms). Two preceding final-note repeats also passed
11/11. This small screen supports compatibility, not a universal quality gain.
Earlier failures and corrected fixture assumptions are preserved in the review.
OpenRouter usage increased by $0.02797 during the focused release checks; GPT
used the existing protected subscription profile in place. No credential was
copied into the review app or committed.

## Shared-engine visual review

All evaluated proposals and failures were retained under the ignored
`.scratch/alpha81-release` directory:

- Claude's final-note corpus: `reviewed-proposals.json`, `proposal-atlas.json`
  and `proposal-atlas/` — 473 records, 318 distinct plots, all 20 overview sheets
  inspected. Historical variants and injected rejections remain labeled.
- Release experiments: `live-production-*.json`, `acceptance-*.json`,
  `final-proposals.json` and `final-atlas/` — 117 records, 32 distinct plots,
  both overview sheets inspected, including failed and superseded trials.

Detailed inspection covered counted upper/full phrases, separated regions,
location-linked pace, fast full-range bridges, asymmetric directional travel,
restricted 20–80 phrases and Creative v2 variation. The final counted patterns
retain two short strokes followed by one full sweep; the bounded pattern stays
within its human restriction. Continuous changes retain variation rather than
becoming fixed patterns. Detailed plots show zero planned velocity/acceleration
knot jumps and close sparse-wire position overlays, with no introduced flat
rest at the phrase join. Wire-segment velocity remains piecewise; plots are
commanded estimates, not carriage telemetry. The OpenRouter changes do not alter motion-engine or transport behavior.
The additional companion-rendering changes are reviewed separately below.
No hardware was connected during this review.

## Initial OpenRouter validation

- Full `go test ./...`, `go test -race -timeout 20m ./...`, `go vet ./...` and
  golangci-lint passed. Final affected tests and lint passed after the guide edit.
- Frontend typecheck, localization audit, build and all 1,081 tests in 132 files
  passed. The canonical embedded bundle was regenerated.
- Installer tests passed under both Windows PowerShell 5.1 and PowerShell 7.
- Pure-Go Windows build passed. With the same stripped footprint flags as
  alpha.80, core plus UI is 24,420,352 B (+6,144 B), canonical UI 2,869,517 B
  (+6,813 B). Shipping builds retain symbols for binary vulnerability checks.
- Browser review at port 50332 confirmed host-order defaults and disclosure,
  Advanced-tier repair visibility, updated context floor and hint, and draft
  discard. Unsaved test connections were discarded.
- `check-review-llm.ps1` completed a real local Gemma 12B generation. A normal
  text-only chat request then completed in 3,675 ms without repair or fallback;
  LLM motion stayed Off and the simulated engine stayed idle.

Ignored evidence includes `go-test.log`, `go-race.log`, `go-vet.log`, `lint.log`,
`final-affected.log`, `final-lint.log`, `frontend-*.log`, `installer-ps*.log`,
`review-readiness.log`, UI screenshots and the corrected intent audit. Raw model
responses/private analyses are intentionally not published in Git.

## Inclusion and independent review of PR #358

The owner subsequently requested #358 in this same update. Its original commits
are retained as a merge parent so the PR remains traceable. Translation conflicts
were merged by source key and the single canonical UI was rebuilt from combined
source. No legacy hashed bundle is retained.

The review agrees with shortening the vibrator's speed window/smoothing and
lowering its speed-band floor: the old output was nearly constant after motor
quantization. Max remains an absolute ceiling, zero remains exact, and the
100 ms owner cadence is unchanged. COYOTE uses the same speed-band scale capped
by its earlier saturating curve, a faster release, and the existing user-only
strength and Stop path. Band changes reconfigure shaping without rewriting
unchanged hardware ceilings. No second motion engine is introduced.

Two context corrections were added: Simple now receives facts only for the
selected follow profile, and every tier distinguishes a zero allowed ceiling
from a reported zero strength. Initial expanded 12B trials preserved motion but
answered all six profile/ceiling explanation questions incorrectly. Replacing a
conditional menu of profiles with the backend-selected facts corrected all six
in the follow-up; this is backend state selection, not language classification.

Fresh production evaluations: 12B Intermediate, Terra Advanced and DeepSeek's
new planning tier each pass 18/18 control-contract cases. All replies were read;
profile and zero-ceiling explanations were correct after the change. 12B still
uses occasional loose wording such as calling a connected but silent box
"active"; it also explicitly says output is silent and does not claim to change
strength. GPT's independent vibration screen passes 26/26. These are small
utility-voice screens, not broad model parity claims. A fresh E4B runtime launch
was blocked by automatic approval review with only "blocked by policy"; its
prior 23/24 evidence is retained, not described as a new validation.

A separate inert comparison feeds the shared planner/buffered sampler into
alpha.80 and combined vibrator renderers and the combined COYOTE renderer:
12 scenarios, saved bands 1–100 and 20–80, speeds 25/45/70, ordinary and narrow
upper travel, Max 70%, 20-step motor. At band 1–100 and speed 45, output changes
from 40–50% to 25–50%; at speed 70, formerly constant 55% becomes 35–60%.
All observed levels remain at or below Max. Both overview figures were inspected;
turnarounds show distinct dips, with no repeated zero/off pulses after fade-in.
COYOTE pulse widths vary by motion beneath unchanged strength. This is simulated
command output, not physical motor response or perceived electrical sensation.
Physical acceptance on the user's vibrator and COYOTE remains outstanding.

The 110 combined live records, including earlier mistaken explanations and
unchanged/no-motion replies, are retained in `combined-proposals.json` and
`combined-atlas/` (11 distinct moving plots, one inspected overview). Other
ignored artifacts: `follow-render.go`, `follow-render.json`, both
`follow-render-*.png`, `combined-coyote-*.json`, and `combined-vibration-gpt.json`.

Combined full Go tests, vet and lint pass; frontend typecheck, localization,
build and all 1,082 tests pass. Focused renderer/transport tests include Stop,
teardown, ceilings, silence reporting and band changes. Combined race testing
and exact-commit CI are required before merging/publication. Combined stripped
core plus UI is 24,433,664 B (+19,456 versus alpha.80); UI is 2,871,754 B (+9,050).
No dependency or extra inference request is introduced.

## Publication policy

Alpha.81 adds one explicit version to the existing reviewed unsigned-alpha
allowlist, with the unchanged Microsoft false-positive case ID. This is not a
claim that new binary hashes have Microsoft approval. Alpha.65 remains excluded;
future versions remain rejected. No signing, Defender, checksum, embedded
identity, installer lifecycle or vulnerability gate is weakened. See
[ADR 0014](decisions/0014-public-windows-signing-gate.md).

Publication requires green PR and main CI, the exact current-main annotated tag,
and the tag workflow's source, build, scan and packaging checks. After publication,
verify the downloaded release assets and run the released portable in isolated
simulated review data with a working local model. Existing installed/user-launched
sessions are preserved; this release review does not authorize real playback.
