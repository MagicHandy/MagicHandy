# ADR 0058: Model capabilities independent of provider

Status: implemented with automated and live-model acceptance, 2026-10-25.

Local deployment does not imply a small model, and a larger context alone does
not imply that a model can plan independent outputs. The owner requested three
explicit local capability levels and parity with external models when Advanced
is selected. Keep provider routing and model-facing capabilities separate.

## Decision

Persist `llm.local_capability_level`, defaulting missing values to Simple.
External connections retain their Advanced contract. Resolve features after
resolving the chat, Autopilot or reviewer role; a local Advanced route remains
local in routing, authentication, resource coordination and diagnostics.

| Local level | Minimum context | Minimum output | Contract |
| --- | ---: | ---: | --- |
| Simple | Existing 16K managed allocation | Existing limit, at least 64 | Existing compact local prompts and motion-follow vibration |
| Intermediate | 32K | 512 | Full local guidance, grouped interactive Layered edits and backend output awareness |
| Advanced | 64K | 1,024 | External motion contract, counted Layered phrases, independent vibration, Auto timing, preparation and response repair |

Simple Ollama requests keep the daemon's existing allocation behavior. The two
opt-in levels send `num_ctx`; managed llama.cpp uses the selected allocation.
An externally managed llama.cpp server must be configured by its owner to match
the application's request budget. Selecting a level raises insufficient context
and output budgets but preserves larger saved values. Downgrading does not
silently discard those values. Advanced local history uses the same bounded
conversation window calculation as an external connection of that capacity.

Intermediate's grouped edits are a JSON dialect over the existing Layered score
and validator. Paired range endpoints and distinct full-slider/full-band geometry
make reach changes less ambiguous. They do not interpret the user's prose,
repair model-selected numbers, prescribe a speed, or invent a motion loop.
Duplicate/conflicting fields reject atomically. Full guidance and examples stay
intact. Live testing favored this dialect for interactive edits, but favored the
compact established grammar for Layered Autopilot; the split is intentional.
Creative v2 keeps its edit grammar and receives the useful continuity guidance.

The backend publishes `ModelRoute.features`. Setup and settings get level choices
and budget floors from backend options; chat renders Auto timing and Prepare
decisions early from the saved Autopilot role, and repair/device help from the
saved chat role. Unsaved drafts never change live capabilities. Advanced local
preparation stays on the existing local request coordinator and yields to chat.
Changing level or context retires prepared work and repair offers.

## Safety and limitations

Capabilities grant no device authority. The shared engine, dispatcher, arming,
Stop sequence and late-result admission remain authoritative. A selected but
unarmed/disconnected vibrator with no other playback output gets a reply-only
continuous contract; ordinary shared validation also rejects attempts to start
that unavailable output. Stop still requires a human to restore live intensity.

Advanced is an explicit opt-in, not a claim about model size or the selected
model's quality. The tested Gemma 12B improved under Intermediate but did poorly
with independent control even at 64K. No automatic promotion based on model name
is introduced. E4B was unavailable for live evaluation; its compact path remains
the default. See the [evidence and remaining limitations](../model-capability-review-2026-10-25.md).

No new dependency, transport, engine goroutine or raw device command is added.

Setup follow-up: the owner requested Intermediate automatically for Gemma 12B.
The backend catalog now recommends Intermediate for both curated 12B builds and
Simple for E4B. Easy, Custom and local-backup setup apply that recommendation to
the selected model, including matching family/parameter metadata from existing
Ollama or managed imports. Unknown models stay Simple; names alone do not promote
them. Saved or manually selected capabilities win, and higher budgets are kept.
