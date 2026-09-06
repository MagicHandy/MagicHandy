# Continuous request validation review — 2026-09-06

## Finding

The reported `creative v2 changed pace without a pace request` and
`the response changed motion outside the current request` errors are often
false rejections from host-side language heuristics. The released source
rejected eleven exact, otherwise valid Gemma 12B edits in the 37-case core set.
Examples include reducing the overall rate by a quarter, a Spanish speed
request, changing width without changing speed, and changing width followed by
an explanation. Two other Gemma rejections correctly prevented paused edits.

The replacement uses model-selected actions and a backend-state grammar
([ADR 0025](decisions/0025-continuous-chat-intent.md)). No phrase lookup,
negation stripping, or per-word speed/coverage permission remains in these
interactive continuous-mode checks. Numerical validity, state transitions,
controller ownership and Stop still belong to the backend.

## Live comparison

Local llama.cpp b9966 CUDA, one GPU worker at a time, RTX 5070 Ti, 16,384-token
context, one slot, thinking disabled, existing chat sampling settings. Models
were already installed; no credentials or user conversations were copied.
The new E4B file is `gemma-4-E4B-it-heretic-Q4_K_M`, SHA-256 prefix
`d0027dd3a912`. Gemma 12B uses the existing `1bdd95189f67` weights; Granite
uses the existing Huihui 3B `25034e5e49d8` weights.

Each of 37 core cases starts from an independent score at 40% speed, within
saved limits 10–80. Checks verify requested values, preservation of other
controls, whether motion was actually authorized, and absence of repair or
fallback. Fixed fixture seeds make previews reproducible; production seeding
and motion variability are unchanged. The set deliberately targets known
failure modes and is not a population estimate of user error rates.

| Model | Released source: accepted / 37 | Released source: intent / 37 | Final harness: accepted / 74 | Final harness: intent / 74 |
| --- | ---: | ---: | ---: | ---: |
| Gemma 12B | 24 | 24 | 74 | 74 |
| Gemma E4B Heretic Q4_K_M | 20 | 19 | 74 | 72 |
| Granite 3B | 27 | 20 | 70 | 51 |

Final results are two fresh generations per core case, not a retry of failed
responses. The initial malformed reply and any accepted request mismatch stay
visible in the reports. The core Gemma 12B mapping checks did not regress.

## Alternatives evaluated

- Action contract with a flat schema: Gemma 12B 37/37 intent; E4B 36/37.
- Shorter decision prompt: Gemma 12B 37/37; E4B 35/37, including a contradictory
  no-change reply. Keep the fuller semantic guidance.
- No schema guidance: Gemma 12B 35/37; E4B 0/37 structurally valid because it
  omitted the required action. Structured generation matters on this runtime.
- A generated intent summary in the same response: E4B 29/37 intent. It
  increased unwanted edits in ordinary conversation. Rejected.
- A separate intent pass: E4B 31/37 intent and additional latency. It still
  misunderstood quoted instructions. Rejected.
- An extra explanation of layer-period versus speed units: E4B remained
  72/74. It did not solve pace-layer confusion, so the paragraph was removed.
- Optional reasoning on eight difficult E4B cases: 2/8 intent. Defaults were
  preserved. This small diagnostic subset does not measure general reasoning
  quality.
- Coupling `none` to empty edits in the schema preserved the 36/37 E4B core
  result and increased exploratory holdout acceptance from 24/29 to 28/29.
  This mechanical consistency check uses output structure, never chat words.

## Limits and full-app evidence

The separate 29-case holdout includes quotes, hypothetical starts, stopped
adjustments, compound explanation requests, layer removal, and mixed reach.
Gemma 12B accepted 29/29 and met 27 strict intent checks; E4B accepted 28/29
and met 23. Some wording is ambiguous: fixing the current location versus
preserving existing roaming is one example. Other failures are clear: a reply
can claim base rebounds without selecting base focus and mixed reach.
The exact released source accepted and passed 14/29 of the same Gemma holdout
turns; its parsed proposals met 22 checks before host authorization.

E4B still confuses overall pace with the pace variation layer, can misread
quoted suggestions, and can emit contradictory layer or geometry edits.
Its full-build 16-turn conversation accepted 15 turns and met 12 sequential
checks; a rejected start also affects subsequent turns. Independent fixtures
above separate those cascades from individual request mapping.
Gemma 12B accepted 16/16 full-app turns and met 14 checks on both the released
and candidate builds. Omitted pace and location edits remain possible in a
compound conversation; the change does not claim to solve every model miss.

Full-app tests use the normal SSE chat path, saved provider settings, the
shared engine, and an explicitly isolated simulated transport. They do not
connect or command a physical device. The trace includes target application,
retargeting and Stop. A plotted command estimate does not establish physical
feel or measured device response.

## Visual review and verification

The final atlas retains 1,044 results from 28 reports, including rejected
experiments and accepted mismatches: 925 compiled entries, deduplicated to 108
plots across seven overview sheets. All overview families and detailed pace,
direction-timing and mapping outliers were inspected. Equivalent scores share
one plot; its title names the first representative and the manifest records
the other trials. For an inactive or reply-only test, a current-score preview
is an inert reference, not evidence that playback started.

Pace-only changes preserve the spatial contour. Directional timing changes
the phase portrait and can lower achieved mean travel at the same speed
setting because the shared kinematic limits still apply. Planned and
whole-percent wire excerpts track each other. No compiled entry exceeded
semantic 0–100 position or its reported device peak-velocity limit. The largest
planned acceleration discontinuity at a knot was 0.000294%/s²; the maximum
finite-segment jerk estimate was 149,994%/s³. These are commanded estimates.

The plots also exposed a weak E4B choice: `mix_percent:1` produces nearly
uniform full strokes, with stroke-length CV 0.005, despite a reply describing
short base strokes and rebounds. A Gemma mixed-base miss instead selected
full-only reach, CV 0.000. Neither is treated as evidence of good natural
motion. The full-app evaluation now explicitly checks mixed reach as well as
focus, and regraded copies retain the original replies and earlier scores.

Validation passed: full Go tests, vet, full race suite, golangci-lint, pure-Go
release build, and frontend typecheck/build with all 473 tests in 64 files.
The committed frontend bundle is unchanged. State tests include a provider
ignoring the schema, attempted paused resumes, contradictory holds, absent or
invalid actions, and a saved speed-limit violation. Full HTTP tests retain
mode fences, shared-engine dispatch and Stop coverage.

The review app runs in real-device mode with isolated data, no configured
device key, and the available Gemma 12B worker. The readiness script completed
a real generation; a text-only app chat returned `Ready` without motion,
repair or fallback. [Performance measurements](perf-baseline.md) record a
512-byte binary increase and no new dependencies or browser assets.

## Reproduction and artifacts

Use an installed local llama.cpp worker, then run:

```powershell
$env:MAGICHANDY_EVAL_URL = 'http://127.0.0.1:8487'
$env:MAGICHANDY_EVAL_MODEL = '<loaded-model-alias>'
$env:MAGICHANDY_EVAL_REPEATS = '2'
$env:MAGICHANDY_EXPERIMENT_CAPTURE = '.scratch/continuous-request-validation/repeat.json'
go test -tags liveeval ./internal/chat -run '^TestContinuousRequestLive$' -count=1 -v
# Set MAGICHANDY_EVAL_SUITE=holdout for the separate exploratory set.
# MAGICHANDY_EVAL_HARNESS supports production, compact, and unguided.
```

The evaluation produces reports rather than failing the test executable on
model mistakes. Inspect `valid`, `intent_pass`, raw replies and proposed scores.
Unit and integration tests independently enforce backend invariants.

Raw reports, failed experiments, exact baseline/current test executables,
full-app traces, build logs and visual artifacts are local ignored evidence
under `.scratch/continuous-request-validation/`. None are shipping assets.
The baseline is `origin/main` at `2eaee13` (alpha.40), tested separately from
the candidate. Rejected experimental harness source is retained there as text.
