# Alpha.77 release review — 2026-10-28

## Ownership and source

The maintainer requested an update and delegated publication to Updates.
The source is the existing `MagicHandy-vibrator-level` worktree and
`claude/vibrator-model-level` branch, based on alpha.76 `43ac5d58`.
The 121 inherited changed/untracked paths include Claude's implementation,
the source thread's fixes, tests, documentation and the one canonical UI.
No unrelated checkout or running user session is replaced.
Remote release/main discovery confirmed alpha.76 and `43ac5d58` before choosing
alpha.77. No managed llama.cpp pin, dependency or release build flag changes.

## Integration review

Reviewed configuration migration, model capability/schema/parser and prompt
projection, stopped-score and previous-envelope context, HTTP admission and
manual-resume rollback, shared-engine scalar sampling, transport output,
Max-at-write and Stop fences, frontend projections and associated tests.
The implementation evidence is in the
[vibrator review](vibrator-level-review-2026-10-28.md),
[ADR 0061](decisions/0061-model-controlled-vibration-level.md) and
[ADR 0062](decisions/0062-retire-original-creative-show-simple.md).

The model chooses only a share of the saved user Max. It cannot select motors,
devices or playback participation, change Max/model control, or authorize a
background resume. Simple Layered and Intermediate use a scalar level;
Advanced uses independently sampled envelopes in percent of Max. Simple
capability in Simple motion remains excluded by evaluation. The existing
100 ms owner pacer computes glide; there is no second motion engine or new
runtime loop. Startup fade and rounded wave/pulse shapes remain; the forced
600 ms Advanced retarget ramp is removed.

Separate human-resume revision protects rollback from model changes and newer
explicit Resume. Successful settings-only echoes remain accepted, but restore
suspension when dispatch has not started playback. Stopped score/envelope data
is context, never authority. Equivalent pace pairs resolve once against the
actual/recalled score. Combined independent edit groups, agreeing geometry and
`vibration:null` retain duplicate/conflict/bounds and non-null authority checks.
The final review found an early Silence status during a scalar glide to zero:
the requested model level was zero while positive motor commands were still
being sent. A regression reproduced 35% commanded output with `level_zero`.
The status now waits for the last commanded output to reach zero; the pacer,
glide, write bounds and Stop behavior are unchanged. The regression checks both
the positive glide and completed zero output.

## Evidence and limits

The source thread retained production-prompt trials: E4B Layered 24/26,
Gemma 12B Intermediate 52/52, GPT 6 Sol Advanced 26/26, and 16/16 real HTTP
simulated-device continuation turns each for Gemma and GPT. Two final E4B
failures and worse discarded guidance/output-budget experiments remain.
Boundary evaluation has 32/32 unrepaired baseline/final requests; 22 controlled
equivalent representations changed from rejected to accepted with identical
commands, and 23 fresh representations also match. Those replays are not a
claim of naturally occurring model failure counts.

Updates inspected all four retained overview sheets, speed coupling at
20/50/80%, all 15 Advanced envelope choices and detailed anchored, continuation
and Creative v2 stronger outliers. The atlas retains 368 decisions/replays and
45 distinct shared-engine outputs, including failed selections. Buffered
commands track the displayed plans; the rejected stronger-only experiment is
retained. Curves are commanded estimates before physical motor response.
Generated plots, raw model replies, logs and runtime data remain ignored under
`.scratch/claude-vibrator-followup`; they are not published as source.

The source thread's full ordinary/race suites passed before the final parser
follow-up. Its final ordinary full run is **not green**: Windows security
blocked the HTTP API test executable and two voice-install child-process
fixtures as potentially unwanted software, including a normal retry. The final
chat/HTTP API race suites passed; other ordinary packages passed. Vet including
liveeval/labs tags, lint (0 issues), pure-Go Windows build, 1,069 frontend tests,
typechecking/localization and canonical UI build passed. No security setting
was changed. Final PR, merged-main and tag CI must establish ordinary full
tests and all remaining release gates before publication.

[The scorecard](goal-scorecard.md) records stripped review core
24,292,864 -> 24,336,896 B (+44,032 B), canonical UI
2,841,582 -> 2,857,145 B (+15,563 B), main JS +1,459 B gzip and unchanged CSS.
These are review measurements; shipping uses the existing symbol-retaining
release flags. No RSS or cold-start improvement is claimed.

## Release acceptance and artifact policy

This is an alpha publication with explicit unresolved physical acceptance,
not a physical-device approval. No hardware connection or motion is authorized
by this handoff. Real vibrator/Edge 2 motor feel, minimum steps, scalar glide,
speed following, Advanced transitions, Stop-to-silence/restart and simultaneous
device failures remain manual acceptance items. Existing Intiface Stop limits
and the installed-app versus matched-run speed discrepancy remain disclosed.
These limitations are material and included in the concise public notes.

Only `0.1.0-alpha.77` is added to ReviewedUnsignedPublic. Alpha.65 remains
excluded, and alpha.78 and later remain unreviewed. The five payload
executables, native x64 loader, non-solid ZIP, three public assets and
data-preserving install contract remain. Trusted signing is still unavailable;
Microsoft's old false-positive case does not approve new hashes. Full quality
gates, source/exact-binary vulnerability scans, Defender of the exact public
directory, manifests/checksums, exact installer lifecycle, clean reviewed
source and current-main tag provenance remain mandatory.

Updates' local architecture, config, chat, motion, transport and vibration
tests pass. PowerShell 5.1 and 7 installer tests pass. The release-policy test keeps
the negative alpha.65 and future alpha.78 cases. The canonical UI totals
2,857,145 B as recorded. No workflow, dependency or optional runtime pin changed.

## Review app

Preserve the original isolated app at `http://127.0.0.1:50305/#/chat`.
The final status correction runs in a parallel isolated current-source app
at `http://127.0.0.1:50306/#/chat`,
with `-simulate-motion`, remote listener disabled and local Ollama Gemma 12B
Intermediate. The source thread's real-generation probe and text-only reply
passed without repair/fallback/motion; compound edits, Stop, a question and
unchanged continuation preserved exact FlowSpec and vibration level. Both
in-process simulated devices are selected, Autopilot is off and playback is
stopped. Updates revalidates this exact app and leaves a review tab open before
handoff. Updates' fresh PowerShell 7 generation probe succeeds on both apps;
text-only app chat is also checked on the final build without repair/fallback
or motion. Both have fresh isolated data and no credentials or user data are copied.
