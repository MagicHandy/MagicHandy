# ADR-0015: Director/Actor Chat Latency Architecture

## Status

Accepted

## Context

Procedural chat previously used a single LLM call returning dialogue + `MotionCommand` JSON. Local llama.cpp processes requests sequentially, so motion waited for the full model response (often 2–7s). Users perceive latency as time-to-first-movement (TTFP), not time-to-full-reply.

Feature 002 introduces semantic zoning and a Director/Actor split to decouple hardware dispatch from dialogue generation.

## Decision

### Director/Actor pattern

1. **Director** — Fast constrained JSON call returning `semantic.LLMIntent` only (`action`, `location`, `intensity`).
2. **Go resolver** — `ResolveMotionBounds` maps intent + `MotionPreferences` to stroke bounds; `MotionCommandFromLLMIntent` builds procedural physics.
3. **Motion dispatch** — `dispatchChatChaoticMotionAsync` starts `manualqueue.Player` before dialogue completes.
4. **Actor** — Second LLM call streams roleplay text with dynamic system prefix describing current physical state.

### Feature flag

`settings.llm.director_mode` (default `false`). Legacy single-call path unchanged when off.

### Bridge filler

`chatChaos` bridge watcher (Rule 05) maintains continuity between dispatches.

### SSE events

New `intent` event precedes `motion` and actor `delta` tokens.

## Consequences

### Positive

- TTFP bounded by director JSON latency + Go dispatch (~50ms), not full dialogue length.
- Stroke math is deterministic in Go; LLM cannot hallucinate invalid ranges.
- User zoning preferences apply consistently via `MotionPreferences`.

### Negative

- Two sequential LLM calls per turn on single-slot local runners (actor starts after director).
- Dual code paths (legacy vs director) until migration completes.

### Neutral

- Device acceptance still requires Rule 04 sync tests with bridge filler enabled.

## Related

- [Rule 05](../domain_rules/05-procedural-bridge-filler.md)
- [Rule 06](../domain_rules/06-semantic-intent-zoning.md)
- Feature 002 task docs
