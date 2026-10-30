# Alpha.81 independent release review — 2026-10-29

## Scope and disposition

Reviewed Claude's uncommitted `claude/deepseek-parity` work against alpha.80
`1f89a3b9e9bb50b6685bd89b7c1fb11da03daca0`, preserving its source worktree and
capturing the patch in ignored local evidence. The release branch is
`codex/release-alpha81`. PR #358 (follow speed and COYOTE) is separate and is
not included. No dependency, llama.cpp runtime, worker or transport change is
part of this release.

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
commanded estimates, not carriage telemetry. No motion-engine or transport
behavior was changed and no hardware was connected during this review.

## Validation

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
