# ADR 0024: Native Creative v2 stroke generation

- Date: 2026-09-05
- Status: Implemented for review; no merge or release authorized
- Extends: ADRs 0002, 0015, 0019 and 0023

## Context

The original Creative contract can describe reach and four range textures, but
cannot directly express uneven travel, a late velocity crest, or shrinking
rebounds at an anchor with occasional returns to broad strokes. Modulation
layers offer useful variation but superimposed position waves can create
incidental reversals. More texture names alone do not provide these independent
controls. Ordered sections ask small models to author a choreography and tend
to repeat an obvious sequence.

## Decision

Add `creative_v2` as a separate main-chat motion mode, labeled **Creative v2**,
and as the same test contract in LLM Lab. It occupies a reserved cell in the
existing three-column, two-row selector, leaving one blank. Existing saved
defaults and modes remain valid. Model Settings exposes all five modes.

`GestureSpec` describes focus position, local width, local/full mixing, faster
direction and contrast, inertia, rebound count/retained width, and variation.
An optional Gesture field on `FlowSpec` carries that semantic content through
the existing `FlowTarget`, prepared plan, shared sampler, sanitizer, retargeting
and transport. It owns no motion goroutine, clock or raw device payload. A nil
Gesture retains the existing continuous Flow compiler. Mode admission rejects
a mismatched score, including a reply arriving after the mode changed.

The generator creates 32 primary cycles by default. Seeded groups vary local
excursions among full strokes, with a broad stroke after at most six local
primary cycles for an intermediate mix. Rebounds contract geometrically at
the chosen anchor. Tails below 10 percentage points are omitted. Rebound count
is an upper bound, and mix is a preference rather than an exact fraction of
elapsed time. Bounces add cycles to local groups, so dense settings can make
the local work dominate. All destinations stay inside the outer requested band.

Each stroke uses a monotonic time warp of a minimum-jerk quintic primitive.
Inertia moves the velocity crest later within travel; it does not simulate
force, collision or accurate ball physics. Quintic Hermite intervals preserve
position, velocity and acceleration at knots. The actual interpolant is
checked for finite coefficients and unintended reversals using exact velocity
extrema. Stroke timing is fitted locally, so a short rebound does not slow the
whole score. When the faster direction saturates, the slower direction retains
its proportionally longer floor. Creative v2 uses the existing runtime bounds
of 7,500 %/s², 150,000 %/s³ and the calibrated velocity/reversal limits. No shared
safety constant changes. Historical Flow keeps its 2,400/24,000 authoring fit.

The model emits a reply plus an order-independent list of one-control edit
items. Related fields form complete groups; omitted groups persist. The list
is an atomic transaction, not a temporal sequence. Unknown fields, duplicates,
partial groups, nulls and invalid bounds are rejected. This shape avoids a
cross-group property-order dependency seen with constrained object generation.
Both Gemma and Granite use the same prompt; a smaller-model gain cannot justify
a Gemma regression. Explicit pace-preservation and selected coverage checks
reject contradictory output without inventing substitute motion. These checks
are not a complete natural-language intent verifier.

Fresh initialization and explicit `evolve` choose a new nonzero realization
seed; the model never selects a seed. Captured scores remain reproducible.
Production and Lab Autopilot refresh the realization while preserving every
gesture control, pace and outer band. Exact-hold requests take priority.
Continuation still goes through one ordinary inference and conditional engine
retarget, with no repair or library fallback. The finite score repeats when it
is not refreshed. Lab scheduling adds jitter above the configured minimum quiet
interval. Preview-only Autopilot does not start a transport.

Original Creative gets a smaller optimization: an intentionally authored new
phrase or texture gets a fresh realization, while speed/horizon-only edits and
saved-score replay retain their existing phrase. Its prompt and texture
definitions are unchanged; the new travel vocabulary belongs to Creative v2.

## Consequences and evidence

This adds an independent motion vocabulary without a second playback path or
new runtime dependencies. The model has fewer compositional responsibilities,
but short stroke speed saturation, finite repeat periods, dense local groups
and model omissions remain visible limitations. Plots establish commanded
character, not physical comfort or device tracking. Stop, controller ownership,
transport boundaries and user limits retain their existing authority.

See [the Creative v2 review](../creative-v2-motion-review-2026-09-05.md) for
all prompt iterations, failed production selections, shared-engine plots,
fake-transport traces, tests, budgets and remaining physical acceptance.
