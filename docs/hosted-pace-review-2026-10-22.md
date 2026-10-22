# Hosted pace descriptions and compound motion

The installed-session review found a low selected pace described as fast and a
compound request that retained locations more faithfully than their relative
tempos. The ceiling in that session was 22%; exceeding it would not be a valid
fix. See the [WAN investigation](wan-session-review-2026-10-22.md) for the
separate latency and connection findings.

## Implementation

Hosted Layered and Creative v2 prompts distinguish the full device speed scale
from the permitted range. Reaching a low ceiling does not make that setting
fast. Replies describe the selected pace, distinguish a relative increase, and
can explain a limiting cap. There are no speed categories, preferred targets,
word lists, reply rewriting, or additional model passes. The existing
constraint-lifetime policy still decides what a person has asked to retain.

Layered's old pace layer could reduce its base travel rate only to one half.
At high settings, tight strokes can reach the smooth-turnaround limit in both
halves of that envelope. Different numeric settings then produce little actual
tempo contrast. A prompt-only coordination candidate exhibited this limitation.

Two optional semantic controls extend the same shared Flow compiler:

- `low_rate_percent` on a pace layer selects its low rate as a fraction of base
  travel rate. Values 1–100 remain finite; zero/omission preserves the historical
  half-rate envelope. `amount_percent` still blends modulation depth. This is a
  rate ratio, not an absolute speed setting or a transport command.
- `inverted` reverses the modulation envelope exactly. Shifting an `alternate`
  envelope by half a period does not invert its unequal dwell times correctly.

The hosted pace operation `follow` takes a source axis (`center` or `range`),
relation (`same` or `opposite`), depth and rate floor. The domain applier resolves
it after geometry and all other layer edits, copying their periodic timing into
the immutable score. Missing/inactive sources, drift sources with independent
noise, and competing timing fields are rejected atomically. Field order cannot
change the result. The score uses the normal compiler, sampler, transitions and
Stop. Following composes one score; it does not install a permanent constraint
or another scheduling loop. Subsequent model turns can develop or replace it.

Local prompts and advertised schemas retain their previous bytes. Existing
scores omit these fields and retain their clock/geometry. API providers using
the hosted contract receive the same capability, but live validation here used
ChatGPT.

## Evaluation

Synthetic technical motion requests used real ChatGPT sign-in, `gpt-6-sol`,
reasoning off. Credentials remained in their protected host store. No physical
device was connected or commanded. Starting scores/seeds were held fixed;
speed ceilings varied across 13, 22, 71, 79, 83 and 91. Every failed proposal was
retained. This follows the representative-fixture and iterative-evaluation
approach in [OpenAI's prompting guidance](https://developers.openai.com/api/docs/guides/prompt-engineering).

Compound checks compare completed half-strokes wholly in the lower or upper
region, excluding travel between regions. The requested faster region must have
at least 25% greater mean commanded travel rate. This is an evaluation criterion
only; production does not classify language or force that ratio. Reply wording
and geometric fidelity were also read manually.

| Check | Final result |
| --- | --- |
| Layered, 12 cases repeated twice | 23 valid replies, all 23 passed; one provider timeout |
| Capped-speed timeout recheck | Passed; the related compound capped-speed recheck also passed |
| Opposite-end fast/slow requests, both directions and a paraphrase | All eight completed final cases passed; rate ratios 1.42–3.57 |
| Keep pace/texture while alternating locations | Both repetitions preserved the settings |
| Creative v2: describe pace, capped increase, unrestricted increase | 3/3 passed |
| Production HTTP chat with fake transport | Text greeting, compound start, reversed contrast, replacement with full strokes, and Stop passed; one playback stream |
| Autopilot after releasing an old fixed-pace request | 25 → 31 → 37 → 29 → 23 |
| Explicit lasting pace request | Kept 25 across four continuations while developing shape |
| Open continuation | Moved up and down; one provider timeout; fresh four-turn recheck passed at 29 → 24 → 30 → 26 |

Successful replies in the final Layered batch had a median of **4.105 seconds**.
The two provider timeouts used the evaluation's 60-second deadline and remain
recorded failures; their targeted rechecks passed. The change does not claim to
eliminate provider outages or every possible model interpretation error.

Capped replies described a low pace, kept the pace without calling it fast, or
said the increase reached the saved limit. Final selections across the varied
requests ranged from 11 to 70 rather than converging on 22. This is an observed
sample, not a new preferred range. Earlier iterations include mismatched timing
shapes and inappropriate band narrowing; these failures were not discarded.

## Visual and numerical review

Ignored artifacts under `.scratch/wan-session-review/` retain the raw synthetic
responses, rejected/admission-failed iterations, compiled scores and fake-device
capture. `pace-all.json` and `pace-http.json` generate `pace-atlas.json` and
`pace-atlas/index.html`: **200 records, 121 distinct plots, eight overview
sheets**, including 36 compiler boundary/inversion fixtures. Private installed
chat and credentials are not in these artifacts or the committed repository.

The overview exposes the rejected candidates: location alternation without
tempo contrast, opposite timing, and motion confined to an unintended middle
band. Final outputs retain both regions and place faster half-strokes at the
requested end. Width drift remains visible; even-pace cases retain their old
texture. Detailed review covers low-capped/wider-limit compound output, reversed
contrast, failed band selections, extreme floors across all three Handy profiles
and the start/retarget/Stop capture. Extreme floors can create long slow phases;
they are boundary cases, not defaults.

Tests cover legacy clock equality, bounded position/derivatives, exact
inversion, lower live limits, atomic follow/clear operations, source resolution
after all edits, strict provider serialization and unchanged local contracts.
Normal teardown gates remain enabled. These are commanded estimates and fake
dispatch, not carriage telemetry or physical comfort validation.

Reproduce with the opt-in `liveeval,magichandy_labs` tags, an existing signed-in
profile in `MAGICHANDY_HOSTED_DATA_DIR`, model `gpt-6-sol` in
`MAGICHANDY_LIVE_MODEL`, and ignored `MAGICHANDY_EXPERIMENT_CAPTURE` destinations:
`TestHostedPaceLiveComparison` / `TestHostedCreativePaceLive` in `internal/chat`
and `TestHostedPaceLiveProductionConversation` in `internal/httpapi`.
`MAGICHANDY_PACE_VARIANTS=candidate` selects the shipping contract;
`MAGICHANDY_EVAL_REPEATS` controls repetitions. The baseline comparison removes
the additional pace instructions/schema choices before calling the provider.
Use `TestConstraintLifetimeLive` with `MAGICHANDY_CONSTRAINT_MODES=layered`,
cases `open,released_speed,lasting_speed`, variant `current`, and live context
`131072` for the continuation probes. Export the retained JSON through
`cmd/motion-atlas -catalog=false -llm <reports>` and render with
`scripts/render-motion-atlas.py`; include the HTTP capture with `--captured`.

## Review build and related CI correction

The isolated simulator at
`http://127.0.0.1:50262/#/settings/access/sessions` shows the eight-hour idle
default. The rebuilt app passed `check-review-llm.ps1` against local Gemma 12B and
returned a nonempty text-only chat reply in **295 ms**, with motion off and no
repair/fallback. The installed application remains untouched.

CI also found an existing Intiface vibrator shutdown race: a copied selection
shared the pacer's mutable motor slice. Stop/close/failure now clone that slice
under its lock. Dispatch behavior is unchanged; a regression checks that a
completed send cannot mutate an unlocked Stop snapshot. No gate was weakened.
