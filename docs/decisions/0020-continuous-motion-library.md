# ADR 0020: Continuous motion recipes replace the legacy selection catalog

- Date: 2026-09-04
- Status: Accepted implementation; physical feel remains under evaluation

Availability update: [ADR 0021](0021-optional-labs-in-release-builds.md)
supersedes this document's development-only Labs boundary. The library decision
remains current; Labs can now be enabled in Settings in every release.

## Context

The user asked to deprecate the current library after repeated poor motion feel
and weak mapping from natural-language requests to actions. Visual review of all
81 existing built-ins found many variations on straight travel legs, repeated
endpoint changes and broad resets. Increasing catalog size did not reliably
increase the number of distinct movement behaviors. Previous experiments in
Creative also coupled range variation, timing and speed fitting.

## Decision

Ten continuous recipes become the active built-in selection catalog: full,
lower, middle and upper strokes; base, tip and centered range variation;
a traveling fixed-width window; alternating wide/narrow sections; and a pace
wave with full reach. These names describe geometry or timing independently of
the pace setting. They compile through the continuous carrier developed in
ADR 0019, without DynamicDefinition or legacy PCHIP authoring normalization.

The compiler produces immutable C2 position/velocity/acceleration content in
the existing MotionPlan. The shared engine still owns startup, retargeting,
quantized sampling, sanitization, limits, dispatch and Stop. The transport
interface and device mapping are unchanged. Exact derivative checks replace
the legacy point-authoring timing rules for these recipes; runtime safety tests
still cover both catalogs and transitions between them.

All 81 prior built-ins are tagged deprecated, hidden by default in the browser
and excluded from model selection and default mode pools. They remain manually
playable, with saved names, weights and enabled preferences preserved. User
patterns retain their existing behavior and experimental capability gate.
Default semantic motion now resolves to Full sweeps. The saved choice between
Creative, Pattern Library and Off is not migrated or overwritten.

Public builds include the new library and its shared continuous compiler.
Motion Lab, LLM Lab, experiment prompts, trial handlers and atlas tooling remain
development-only. This refines ADR 0019's initial experimental boundary; generic
FlowTarget/compiler symbols are now expected in a public binary. Callable lab
handlers and UI routes must still be absent.

The model receives ten behavior descriptions with opaque catalog handles.
Live evaluation compared opaque handles, descriptive IDs and action names.
Opaque handles performed best in the interactive production contract; the
separate compact Lab contract allows all three variants because its results
differ. Provider schemas constrain structure and available choices. They do
not infer intent, authorize motion, or fill missing semantic fields. Interactive
chat and autonomous planning receive examples for their own response contracts.

Every evaluated recipe and accepted LLM proposal is rendered from actual
shared-engine output, alongside raw failures. Visual review is documented in
`docs/motion-visual-review.md` and accompanies numerical checks and eventual
physical feedback. Visual smoothness is not proof of comfortable device motion.

## Consequences

The active catalog is smaller and distinguishes independent behaviors. New
variation uses a seeded continuous field and bounded modulation rather than
adding more named textures. Finite deterministic loops still repeat, and the
sinusoidal carrier may still feel too regular. Conservative acceleration and
jerk budgets can reduce achieved pace, especially for narrow strokes.

Continuous library previews are sampled from the actual plan at a labeled 25%
reference pace. The existing v1 share format exports a dense sampled bake,
not a portable recipe. Reimported bakes use the existing editable-point path;
they are approximations and do not retain the recipe's derivative contract.

The LLM Lab supports bounded control edits, up to four ordered sections and
three parameter-modulation layers. It never combines arbitrary position
streams or commands hardware on generation. Small-model errors remain visible;
no model result here establishes general reliability or physical acceptance.

See `docs/motion-library-review-2026-09-04.md` for evidence and limitations.
