# Model response reliability review — 2026-10-23

This extends the [slow Layered review](layered-slow-turn-review-2026-10-23.md)
with real provider, parser and shared-compiler tests. It does not connect to
hardware or change the installed session. Physical acceptance remains deferred
at the user's request and is required before publishing the motion changes.

## Findings and adopted changes

**An autonomous change could discard a lasting travel restriction.** In the
initial GPT-6.1 Sol Low screen, one of 36 Autopilot decisions replaced the
person's explicit 20–70 travel restriction with 0–100. The model chose a valid
upper/lower pattern, but that pattern did not respect the person's boundary.
The contract distinguished current coverage from a limit without making the
source of that coverage clear enough. It also advised against keeping earlier
constraints permanently. The revised hosted contract distinguishes a
model-selected band from a human restriction. Autopilot's permission to develop
open dimensions does not itself release a restricted dimension. A later human
request can release it, and a one-time adjustment need not become a lasting
restriction. No language classifier, inferred numerical clamp or fixed expiry
is added.

**A failed request could influence a later edit as if it had applied.** A
transient provider failure occurred during a sequential conversation. The
failed turn requested slower full sweeps. Two turns later, a speed-only edit
also installed that earlier slowing while claiming the group rates were
unchanged. The revised contract names `current_score` as applied motion and
distinguishes it from historical requests or claims. New fixtures exercise
unanswered requests, an obsolete assistant claim, a failed range change, and an
explicit retry. Preserving current state must not prevent a deliberate retry.

**Local stopped Layered chat unnecessarily asked for confirmation.** A small
instruction now distinguishes performing a requested rhythm from explaining
one. It is included only in local Layered interactive chat while stopped,
before the existing final output guard. Active local motion, local Autopilot,
Creative v2, original Creative, Pattern library, hosted prompts and all start
authorization checks retain their existing paths. No additional generation is
introduced. The existing minimum-context and voice/guard checks remain green.

## Real-model evidence

Cloud calls used the host's existing ChatGPT credential source without copying
credentials: `gpt-6.1-sol`, Low reasoning, 131,072 context, and the production
contracts. Fixtures construct no engine or transport. Every response and
rejection is retained under the ignored `.scratch/response-depth-61/` directory.

The final focused confirmation, after the hosted contract fixes, passed
**106/106 intent and validity checks**:

| Final confirmation | Checks | Median request time |
| --- | ---: | ---: |
| Sequential pattern conversation, two repetitions | 20/20 | 5.081 s |
| Full-device/restricted/upper/lower coverage, two repetitions | 24/24 | 6.003 s |
| Replacement timing and deliberately slow groups | 12/12 | 5.740 s |
| Temporary, lasting and released constraints, with actual chat acknowledgments | 26/26 | 8.121 s |
| Unapplied history and explicit retries, three repetitions | 12/12 | 4.997 s |
| Automatic/manual response repairs | 12/12 | 6.428 s |

Repair fixtures deliberately inject the initial rejected JSON. The replacement
is a real generation through `Service.Repair`; the injected draft is not a
natural model failure or a successful generation. Each repair is bounded to
one additional call. Ordinary final conversation turns require one call,
nonempty replies and no repair or fallback.

The preceding independent screens also passed 8 operation/reviewer cases,
25 lifecycle cases across four modes, 5 phrase-join choices, 12 pace cases,
7 interactive Autopilot-hold decisions, and 8 story-continuation cases with
approximately 72 KB of history. The story retained revised names, possessions,
meeting place and a motion agreement, then respected paused/stopped states.
Those screens are supporting evidence, not additional reruns of the final
contract. One intermediate conversation retained the provider failure and its
subsequent semantic miss; the final complete conversation passed 20/20.

The first expanded constraint run with actual acknowledgments passed 46/46.
In it a temporary speed adjustment eventually developed from 25 to 30 and 27,
while an explicitly lasting speed remained 25. A lasting 20–70 band remained
bounded; an explicitly released band returned to 0–100. These are observed
choices, not preferred values or schedules inserted into the product.

## Local comparison and rejected approaches

Local tests used the available Gemma 12B Ollama model
`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`, 16K context, reasoning
off and 256 output tokens. The final production path improves the existing
difficult matrix from **31/40 to 32/40**, median 809.5 to 800.5 ms. That timing
difference is too small to claim a throughput improvement. Its actual gain is
the stopped Layered performance request becoming a valid one-call start.

All **72 expected inert outcomes** across the main matrix and added holdouts
remain inert. The extra 24 negative cases pass 24/24; the mixed 40-case holdout
passes 32/40. The eight holdout misses are intended starts in legacy Creative
and Pattern library that the existing authorization checks remove.

Eight known semantic misses remain in the main matrix: four legacy starts,
two mixed full-device/regional requests that omit outer endpoints, and two
regional pace requests that select the wrong geometry or coordination. These
are not mislabeled as fixed. This review does not extend the hosted counted
phrase grammar to the smaller model.

Rejected candidates remain in the captures:

- Generic hosted start guidance with relaxed local start authorization caused
  false starts on questions and hypotheticals.
- A stronger intent-first guide did better in live fixtures, but removing the
  existing guards also failed the defensive tests for refusing motion and for
  profile/history text that must not grant start authority. That authorization
  change was reverted; the safety assertions were retained.
- Longer regional explanations did not solve omitted bounds and could exceed
  the conservative 16K repair budget. A substantially shorter replacement
  contract worsened geometry/phase selection and fixed-width roaming.

Adding semantic shortcuts for mixed full-device coverage or exposing the
existing layer-follow operation to local models deserves a separate controlled
comparison. Neither is silently enabled here to make this matrix pass.

## Evaluation and visual integrity

The retained set contains **921 evaluation records**, including 34 injected
repair drafts and rejected candidates. It is not a claim of 921 independent
successful calls. The inert exporter uses the real original-Creative dispatch
conversion and shared compiler, rather than treating every mode as a Flow
score. It never constructs a server, transport or playback loop. The atlas
labels inert review explicitly and preserves each record's actual model.

Two fixture defects were corrected:

- The counted-seam repair check required incidental 70/35 endpoints that the
  human request never specified. It now checks regional intent and counts;
  a separate explicitly bounded case still requires those exact endpoints.
- The interactive hold fixture's state used 10–50 speed limits but its export
  recorded factory limits. The fixture now exports its real limits. The old
  file is retained unchanged; the atlas uses a separately annotated metadata
  correction with identical raw model output.

Visual artifacts: `.scratch/response-depth-61/motion-atlas.json`,
`atlas-final/index.html`, `atlas-final/manifest.json` and `contact-01.png`
through `contact-16.png`. All 16 overview sheets were inspected, followed by
detailed figures for deliberately slow sweeps, explicit 90–100 coverage,
unapplied-history preservation, the failed and corrected lasting-band cases,
the remaining local full-device miss, and the explicitly bounded repaired join.

There are 449 compiled records, deduplicated into 249 distinct plots. After
correcting the capture-limit metadata, no accepted moving target fails
compilation. The largest reported planned knot discontinuities are about
0.000015 percentage points/s in velocity and 0.000293 percentage points/s² in
acceleration. These values describe continuity of the generated curve, not
the quantized wire velocity or physical carriage behavior.

The plots show three-plus-one counts retained through timing and coverage
changes, smooth planned loop joins, and slower full sweeps preserved when
explicitly requested. Narrow 90–100 groups remain narrow. The corrected
lasting-band example compiles inside 21.33–68.38, within its requested 20–70
restriction, while the baseline failure reaches 0–100. The remaining local
full-device mixture visibly stays inside its old 20–85 band, confirming a real
intent miss despite valid JSON. Original Creative's syncopation selections
are much more irregular than the continuous alternatives; lifecycle checks
establish start intent, not equivalence of their motion character.

Detailed wire overlays still contain piecewise velocity steps. No claim of
physical smoothness or hardware acceptance follows from these charts.

## Second opinion

A read-only Claude Opus 5.5 Max CLI consultation was attempted with sanitized
synthetic cases and public source. It returned no response after approximately
20 minutes and the task-owned process was ended. No Claude recommendation or
approval is claimed; the prompt and empty result/error captures are retained.

## Validation and release boundary

The ordinary Go suite, `go vet`, `golangci-lint`, the Labs-tagged suite and a
`CGO_ENABLED=0` build pass locally. Race validation runs in CI. The initial broad invocation
with `liveeval` also selected older fixtures that require an unavailable
llama.cpp/managed-model endpoint; those environment failures are retained and
are not reported as model or application regressions. The selected live tests
above ran against the available providers.

The current-source review application uses an isolated simulator profile and
the available Ollama model. Its real text-only chat request completed in
2.834 s, including 2.507 s model load, with one provider call, no repair,
fallback or motion. `scripts/check-review-llm.ps1` completed another real text
generation successfully. User-installed playback and credentials were not
changed. Physical turn quality is still governed by the existing
[pre-update checklist](layered-slow-turn-review-2026-10-23.md#physical-acceptance-required-before-the-next-update).
