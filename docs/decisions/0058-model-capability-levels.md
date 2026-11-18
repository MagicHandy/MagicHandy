# ADR 0058: Model capabilities independent of provider

> Amended by [ADR 0061](0061-model-controlled-vibration-level.md): Simple (Layered)
> and Intermediate set one vibration level under the user's Max; Advanced keeps
> the envelope in % of Max. Only a person lifts Stop's pause: a fresh chat
> request, Resume now, or choosing the device again.
>
> Amended by [ADR 0065](0065-advanced-no-reasoning-and-openrouter-limits.md): a
> fourth level, Advanced (no reasoning), has every Advanced feature plus a
> private planning note before interactive chat edits, for capable models
> answering without reasoning. A connection's level floor now also applies to
> its own requests.

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
| Simple | Existing 16K managed allocation | Existing chat limit; Autopilot motion at least 384 | E4B-tuned local prompts and motion-follow vibration |
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
is introduced. E4B was unavailable during the initial evaluation, then tested
in the [Simple follow-up](../e4b-simple-review-2026-10-25.md). Simple and
Intermediate Autopilot now use [ADR 0060's decision note](0060-local-autopilot-decision-notes.md),
which records the accepted latency cost. See also the
[initial evidence and limitations](../model-capability-review-2026-10-25.md).

No new dependency, transport, engine goroutine or raw device command is added.

Setup follow-up: the owner requested Intermediate automatically for Gemma 12B.
The backend catalog now recommends Intermediate for both curated 12B builds and
Simple for E4B. Easy, Custom and local-backup setup apply that recommendation to
the selected model, including matching family/parameter metadata from existing
Ollama or managed imports. Unknown models stay Simple; names alone do not promote
them. Saved or manually selected capabilities win, and higher budgets are kept.

## Amendment: every model has a level (2026-10-25)

The owner asked for the capability choice on external providers too, in
particular a server such as LM Studio, which may run the same small model as a
local runtime. Resolving every connection to Advanced gave such a server the
contract the tested 12B fails (4/20 independent-control turns), and a 64K floor
its loaded context may not provide. This amends "External connections retain
their Advanced contract" above.

- `ModelConnection.capability_level` selects Simple, Intermediate or Advanced
  per connection. An empty value keeps Advanced, so connections saved before
  levels existed are unchanged. New "Other compatible provider" connections
  start Simple, like an unknown local model; ChatGPT, OpenAI and OpenRouter
  start Advanced. No level is inferred from a model name.
- A connection's request budget uses the same floors: 16K, 32K and 64K, then
  128K and 256K. Below 64K a named connection keeps 64 messages /
  24 KB. Raising a level raises an insufficient budget, lowering it keeps a
  larger saved one, and a saved 32K Advanced connection still moves to 64K.
- Features, prompts and history follow the resolved level, not the transport.
  A Simple connection uses the compact contract without Auto timing,
  preparation, response repair or independent vibration. Routing, credentials,
  readiness checks and the hosted request lane are unchanged.
- The motion second opinion reviews any role below Advanced, local or served by
  a connection, and no Advanced role. Before, it reviewed every local role.

Settings gives each option one home; see the
[settings review](../chat-model-settings-review-2026-10-25.md). Model roles holds
Chat, Autopilot, the second opinion and the repair and declined-request retry
switches. One Models list holds the local model and each provider, each with the
same Capability and Context rows. Reply length moved beside Chat voice, and the
LLM motion mode and its Pattern library options live only on the Chat page.

Validation: config and HTTP tests cover per-connection resolution, budgets,
validation, routing features, the compact contract and budget for a Simple
connection, reviewer eligibility and the motion card's Pattern library options.
A Simple compatible connection to Ollama's OpenAI endpoint completed the
readiness probe and a text-only chat in one provider call (9,956 of 11,633
prompt bytes), without repair, fallback or motion.

## Local conversation history follow-up (2026-11-17)

Interactive chat resolves the message and byte limits once for both storage
and final prompt composition. Advanced local models given a larger window can
therefore receive earlier corrections. Compact reads count omitted committed
messages in the same snapshot as the visible dialogue. The current request and
system guidance are excluded from the count of prior dialogue.

For English interactive Layered motion, a local Simple model receives the
current band and pace when earlier dialogue is missing. A running local
Advanced model receiving more than 64 prior messages gets examples of reading
back a definition and applying it. The examples must fit alongside all the
selected dialogue. The matched local and hosted results explain the scope.
[Multi-turn request evaluation](../motion-request-quality-2026-11-17.md)

## Paired range edits for local Simple chat (2026-11-18)

In ordinary English interactive Layered chat, local Simple and Intermediate
can use the same list of edit groups. Each range group requires both numeric
endpoints. Local Simple retains its own capability, model budget, output
controls and validation. Existing Simple object-form replies still parse. The
interactive schema, prompt inspector and parser resolve the same format from
the current prompt set and playback state. Simple autonomous planning, paused
playback, manual restart, tempo-only outputs and other prompt languages use
their existing formats. [Comparison on the three setup defaults](../default-model-motion-review-2026-11-18.md)

## Overall-speed edits in active hosted chat (2026-11-18)

Advanced English Layered chat offers a short `speed_only` composition while a
Layered field or counted phrase is running on a positional output. Chat
resolves that contract from the actual engine target before seeding a score
for a new composition. The model chooses between it and the existing
compositions. Its single numeric set or adjustment goes through the same
saved limits, lifecycle checks and shared engine as an ordinary control
edit. The parser rejects additional fields, including whole-score recall.
The strict provider can therefore produce a speed edit without generating
nullable fields for stroke geometry and layers.

A running flow also supplies its previous distinct commanded speed to live
chat. It comes from the plan at the accepted handoff, so a replacement at
the same uncommitted cursor cannot become an earlier speed. A new run starts
with empty pace history. The Autopilot planner uses its own existing timeline.
[Installed-history and runtime measurements](../installed-chat-replay-2026-11-18.md)

Local chat compares optional guidance with the dialogue that its provider
actually admits under the selected context limit. The omission count includes
both retrieval and provider truncation. Full and compact guidance can use
available space only while preserving those same turns.

For ordinary English local Layered chat, composition also reserves room for
a complete user message alongside the actual schema, output and reasoning
allowance. If the full reference exceeds that budget, the composer caps the
second copy of recent human lines in the engine snapshot at 1 KiB of encoded
JSON. Complete original turns remain available for normal dialogue admission.
Simple can then use a shorter description of the same grouped edit schema.
The prompt inspector identifies the compact reference. This handles the
16K Simple minimum and unusually verbose history at the 32K setup default.
[Admission and model measurements](../opus-motion-review-2026-11-18.md)

Managed llama.cpp Layered conversations at Simple and Intermediate retain
up to 96 complete dialogue messages within the existing 24,000-byte window
when at least 32K is allocated. This raises the 64-message default that
dropped short definitions from otherwise admissible dialogue. The 16K
minimum uses 64 messages. Named connection history choices and the private
planning context keep their existing behavior. The setup-selected model and
its capability continue to determine the native context and output budgets.
