# What makes a motion pattern good or bad

Status: working guideline (2026-08-13)

Two batches of patterns have now been rejected after being felt on hardware: the
descending/decaying shapes that were hand-authored into the catalog, and the
funscript clips imported as curated built-ins. They failed for the same reason,
and it is not the reason it looks like.

This is the short version of what went wrong and how to avoid repeating it.

## The one thing to understand

The device follows the curve exactly. Patterns are uploaded as a script and
played on a synced clock, so there is no smoothing on the device to rescue a bad
curve.

**What a person feels is speed, not shape.** A pattern's drawing can look varied
and interesting while the thing it actually produces is a device that keeps
slowing to a crawl. Every rule below follows from that.

Shape and global playback pace are nevertheless separate controls. A stored
pattern defines geometry and relative rhythm; the runtime planner measures its
total travel and retimes the whole loop to the requested `speed_percent`.
Catalog names, descriptions, and tags therefore must not promise an absolute
pace such as “fast,” “gentle,” or “intense.” Those words belong to the speed
choice. The rules below ensure that a pattern remains coherent at its reference
pace; a user may still deliberately request a very low playback speed.

## Failure mode 1: the device stops

The dominant complaint, both times, was "stutter" or "stopping". Measured, it is
always the same thing: a stretch of the loop where the rendered curve sits under
about 30%/s. Under roughly 45%/s a stroke stops registering as motion; near zero
it reads as the device having broken.

Three ways patterns arrive at it:

**Shrinking strokes on a fixed beat.** This is what killed the hand-authored
"descending" shapes. Peaks step down 96 → 82 → 68 → 56 while every half-stroke
keeps the same duration, so speed falls with the amplitude. `Cascade` reached a
14-unit stroke in 760 ms — 18%/s — and spent **2.46 s of a 6.6 s loop**
essentially motionless. The musical idea (a diminuendo) becomes, on a linear
actuator, the device winding down and stopping.

> If you want a descent, move the *window* down and keep the stroke length. Or
> shorten the duration along with the amplitude so speed holds. Never shrink
> amplitude alone.

**Captured pauses.** The imported clips are excerpts from scripted scenes, and
scenes contain rests. One 10-second clip had **2 live strokes out of 22** — it
was a pause with a twitch in it. 56% of the import stalled.

**Micro-strokes.** A stroke under about 22% of the range cannot be made fast
enough to feel (see the geometry note below), so it always reads as a hesitation.

## Failure mode 2: nothing settles

The rest of the rejected patterns did not stall, but never established a pace.
Their slowest stroke averaged 33%/s against 62%/s for the ones that were kept,
their fastest-to-slowest spread was 3.4× against 2.3×, and they used 5.5
different stroke lengths against 3.0.

The pattern to avoid is **amplitude and duration both varying, independently and
without repeating**. Every stroke ends up a different length *and* a different
speed, and the body has nothing to lock onto. Compare the ones that work: they
either hold a near-constant pace (`Drift`, `Stroke`) or change it in a phrase you
can anticipate (`Flutter`, `Four-Level Circuit`, `Tease`).

## The trap that caused all of this

**Averages hide holds.** Both mistakes came from judging a pattern by a mean.

The imported clips were labelled by mean stroke speed. A clip that is mostly
stopped has a low mean, so it came out labelled "Gentle" — which reads as
*soothing* and invites the model to pick it. Measured afterwards:

| assigned band | clips | containing a dead stroke |
|---|---|---|
| Gentle | 16 | **100%** |
| Easy | 22 | **95%** |
| Steady | 33 | 70% |
| Fast | 70 | 63% |
| Intense | 30 | 23% |

A perfect gradient. The label was reporting *how much the clip rested* while
reading as *how hard it worked*.

> Judge a pattern by its **worst** stroke, never its average.

### Runtime normalization can expose the same defect

Mean-rate playback does not repair a bad ratio between legs. A 2026-08-14 Cloud
trace showed `Hard and Regular` with continuous, accepted HSP timestamps and no
buffer gap, yet its 100 -> 74 return took about one second at 40%. The imported
curve had assigned that 26-point leg almost as much time as the preceding
100-point leg. Uniform retiming preserved that ratio and turned a brief accent
at the fast source cadence into a perceived stop. `Deep-Partial Sequence` had
the same defect in two long full-range legs. Sampling two cycles also exposed a
360 ms `Tease` slowdown split across the loop seam; its short strokes had shared
the same duration as its full reaches.

All three curves keep their turning positions but derive usable leg timing.
Tests render every non-experimental, non-hold loop through the real planner at
40% and reject any continuous interval longer than 250 ms below 45% travel/s.
The Cloud-resolution wire frame for `Hard and Regular` is checked separately so
sampler simplification cannot reintroduce the apex pause.

## Two facts about the geometry

These are consequences of the hardware budgets, not opinions, and they surprise
people:

**Minimum useful stroke ≈ 22% of the range.** The reversal gap is 450 ms and the
speed floor is about 45%/s, so the shortest stroke that can satisfy both is
`0.45 × 45 ≈ 22`. Anything smaller is either too slow to feel or too abrupt for
the device.

**Long strokes are the fast ones.** The same 450 ms gap caps a stroke's speed at
`amplitude / 0.45`. A 30-unit stroke can never exceed 66%/s. So "a broad slow
sweep answered by a quick little one" is not achievable — it has to be the other
way round.

Also: **fast alternation between big and small strokes is impossible** inside a
bounded range. Turning points alternate direction, so a long stroke must be
followed by another long one to stay in range. Big/small contrast has to come in
*blocks*, not alternation.

## Reaching the stored minimum cycle: repeat, never stretch

Stored routine cycles must be at least 6600 ms. It is tempting to reach that by
scaling all the timestamps, and `mustFitCatalog` does exactly that when a spec is under budget —
which divides every velocity by the same factor and puts the stalls straight
back. `Descending Ladder` was authored crisp at 410–474 ms strokes and got slowed
1.22× to reach the floor.

**Repeat the phrase instead.** It fills the cycle at unchanged speed, and hands
back the repetition that makes a pattern feel intentional.

## The rules, as numbers

Taken from the patterns that survived contact with hardware:

| rule | value | why |
|---|---|---|
| slowest stroke | ≥ 42 %/s | below this it stops feeling like motion |
| shortest stroke | ≥ 22% travel | reversal gap × speed floor |
| speed spread inside one pattern | ≤ 3.3× | wider stops reading as one pattern |
| mean speed | ≥ 55 %/s | `Drift` and `Flutter` sit at 56–57 |
| longest dip (rendered, < 45 %/s) | ≤ 120 ms | see below — this is the one that matters |
| reversal gap | ≥ 450 ms | `catalogMinReversalGap` |
| acceleration | ≤ 3000 %/s² | `catalogMaxAcceleration` |
| stored cycle | 6600–12000 ms | authoring floor plus the manifest ceiling |

Playback may produce a shorter or longer runtime period. The planner targets
`180 * speed_percent / 100` mean percentage-points of travel per second, then
lengthens the period if the selected curve would exceed its acceleration or
reversal-gap safety floor. This normalization is what makes a speed change
meaningful across patterns with very different total travel.

### Measure the loaded, rendered curve — nothing earlier

This rule was got wrong three times, each time by measuring something one step
too early. It is the single easiest mistake to make here.

1. **Authored chords are not the rendered curve.** Points are interpolated with
   monotone cubic Hermite, which drives velocity toward zero at every turning
   point, so a stroke whose *chord* averages 42 %/s still sits below that around
   its reversals. Filtering on chords alone left 62 of 153 clips still stalling.
2. **What you author is not what loads.** `NormalizePatternDefinition` resamples
   and can rescale the cycle. One clip authored as 9109 ms with 32 clean points
   loaded as 10470 ms with 21 points and fractional positions — a 1.15× stretch
   that quietly turned a 42 %/s floor into 36.6 %/s. **Always measure
   `BuiltinPatternDefinitions()`**, never your own arithmetic.
3. **"Stopped" is a lower bar than "not moving usefully".** A 30 %/s gate passes
   a clip that crawls at 35 %/s for three seconds. Use the speed floor itself.

### Why the dip rule is shaped this way

Use the **longest contiguous dip below the speed floor**, not total time below it
and not a lower "has it stopped" threshold.

Total time is not discriminating: every reversal dips, so a busy pattern spends
as much time low as a bad one. Measured, time under 45 %/s is a **median 8% for
the designed patterns and 7% for the imports** — indistinguishable.

The longest dip separates them completely:

| | longest dip under 45 %/s |
|---|---|
| every designed pattern | **45–50 ms** (the reversal itself) |
| `Steady Roll 4`, reported as micro-stalling | **420 ms** |
| `Gentle Drive 1` | **3,490 ms**, 76% of its cycle |

The distribution is bimodal with nothing between about 60 ms and 150 ms, so
120 ms sits in empty space rather than on a judgement call.

## Scripts

**Authoring new patterns — `scripts/pattern-designer.js`.**
Author *stroke velocity* and let it derive the duration, instead of writing
positions and times as two unrelated lists. That is the structural fix: with
independent lists, speed is an emergent quantity nobody chose, which is how a
14-unit and an 84-unit stroke ended up on the same 760 ms. It also reports any
pair of patterns too close in shape to be worth choosing between.

```bash
node scripts/pattern-designer.js
```

**Repairing and filtering imports — `scripts/curated-pattern-labeller.js`.**
Excises dead time without touching geometry (merges strokes too small to feel,
shortens strokes slower than the floor), drops whatever still fails, and gives
survivors geometry/relative-rhythm metadata without changing stable IDs or
points.

```bash
node scripts/curated-pattern-labeller.js
```

It needs a second pass, because the rendered-curve check lives in Go:

```bash
EMIT_STALLING=/tmp/stalling.json go test ./internal/motion -run TestScratchEmitStallingCurated
STALLING=/tmp/stalling.json APPLY=1 node scripts/curated-pattern-labeller.js
```

Both scripts hold the envelope above as constants. They are a design aid; the
gate is in Go.

## What enforces this

- `TestCatalogPatternsHoldTheMeasuredSpeedEnvelope` — every built-in against the
  speed envelope, measured on the rendered curve.
- `TestBuiltinCatalogIncludesGeneratedPatternsWithoutExactTimingExemption` —
  curated clips against the acceleration and reversal budgets.
- `TestGeneratedCatalogMeetsHardwareBudgets` — the same for generated specs.
- `TestLoopSpeedNormalizesMeanTravelAcrossPatterns` — different shapes
  request the same mean travel rate at the same speed.
- `TestLoopSpeedProducesProportionalTempoChanges` — 20%, 40%, and 80% produce the
  expected 1:2:4 tempo progression where safety does not cap it.
- `TestMeasureCurveIncludesLoopSeamReversalGap` — reversal timing includes the
  wrap from the final stroke back into the first stroke.
- `TestFocusedLoopRespectsCatalogAccelerationAndReversalBudgets`,
  `TestFocusExpansionPreservesRequestedTravelRate`, and
  `TestSoftAnchorPreservesRequestedTravelRate` — focus compression, expansion,
  and anchoring preserve the requested pace unless a hardware budget requires a
  slower period.
- `TestBuiltinMetadataDoesNotEncodeAbsolutePace` — model-facing metadata does
  not reintroduce global pace through names, descriptions, or tags.

## What this cost

Of 171 imported clips, **59 survived and 112 were dropped**. Excision recovered a
meaningful share, but most of what went was dropped because the dead time *was*
the content, and no amount of trimming produces a pattern from a pause.

The first curation kept 90 and was still wrong: it gated at 30 %/s, so clips that
crawled between 30 and 45 %/s passed and were reported from hardware as
micro-stalling. Tightening the floor to 45 %/s removed a further 31.

That is the honest yield of importing captured material, and it is why the
designer exists: authoring against the envelope produces patterns that pass by
construction, rather than 47% survivors and a filtering pipeline.
