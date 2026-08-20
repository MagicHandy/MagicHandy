# LLM Motion Control Modes

This document defines how the local LLM can drive the device, how users choose
that control surface, what the engine can do that the model still cannot reach,
and the remaining ideas for widening LLM control. Dynamic motion is implemented
as a selectable alternative to Pattern Library control. Pattern Library remains
the conservative default until Dynamic completes live-provider and real-device
A/B acceptance.

It is grounded in two things: MagicHandy's current code
(`internal/chat/contract.go`, `internal/motion/target.go`) and the reference
app's own hard-won motion-control notes
(`StrokeGPT-ReVibed/docs/motion_control_modes.md` and `ROADMAP.md` items #3,
#12, #15, #16). Where the reference app already reasoned about a direction, we
cite it rather than re-derive it.

## The one invariant every idea must keep

Every LLM control path — present or proposed — routes through the shared
sampler/sanitizer and the transport boundary, and the model never sees
transport details. This is [ADR 0006](decisions/0006-drop-legacy-motion.md)
(one motion backend, one neutral frame) and
[ADR 0002](decisions/0002-motion-transport-contract.md) (semantic intent vs
physical transport), and it matches the reference app's explicit guardrail:

> Avoid designing new LLM modes that directly expose transport details like HSP
> replacement, HDSP position frames, morph duration, or phase offsets. Those
> should remain backend implementation details with tests and trace fields.
> — `StrokeGPT-ReVibed/docs/motion_control_modes.md`

So the model emits **semantic intent** (bounded geometry or an opaque pattern
handle plus speed). Deterministic code compiles that into motion and clamps it
to the user's speed/stroke/limit envelope. Speed and stroke limits stay
engine/transport caps, never prompt-only behavior. Emergency Stop stays
independent of every generation, upload, and playback latency.

## What the LLM can emit today

`internal/chat/contract.go` accepts exactly one `AssistantResponse`: required
user-facing `reply`, optional semantic `motion`, and optional `new_mood` for
interactive non-utility chat. `new_mood` is a strict 17-value reply-register
enum. It is persisted as session diagnostics and shown as backend-reported Chat
state, but it has no representation in `MotionCommand`, `MotionContext`,
`MotionTarget`, or transport dispatch.

One persisted mode selects one model-facing vocabulary. Dynamic and Pattern
Library fields are never advertised together:

| Mode | Model surface | Runtime behavior |
| --- | --- | --- |
| **Dynamic** | speed plus center/span or an ordered named-anchor route, slow variation, and a decision horizon | compiles an ephemeral loop through the shared motion engine; no pattern catalog enters the prompt |
| **Pattern Library** | enabled opaque `pattern_id`, speed, and optional named area focus | resolves an authored library shape through the shared motion engine |
| **Off** | reply text and optional mood only | rejects model motion while every user Stop path remains available |

The Chat Controls sidebar exposes all three modes beside Autopilot. Settings >
Model exposes the same list with the Pattern-only area-focus and
experimental-content gates. The permanent navigation rail remains a list of
destinations. The scoped mode endpoint persists immediately and requires
controller ownership. Changing mode cancels Autopilot planning so an in-flight
decision from the old vocabulary cannot land late; it does not stop
already-running engine motion. Dispatch rechecks the saved mode before applying
a result. `stop` is exempt from that rejection and remains unconditional even
after switching to Off.

`MotionCommand`, the only model-authored motion union the parser accepts:

| Field | Values | Meaning |
| --- | --- | --- |
| `action` | `none` / `start` / `target` / `update` / `stop` | Pattern Library uses `target`; Dynamic uses `update`; both start and stop only through the engine |
| `speed_percent` | 1–100 | semantic playback speed, clamped again by the user's limits |
| `pattern_id` | an opaque **enabled** catalog handle | Pattern Library only: resolve one enabled authored shape |
| `area` | `tip` / `shaft` / `base` / `full` | Pattern Library only: select a named stroke zone; `full` clears focus |
| `center_percent` | 0–100 | Dynamic only: midpoint of the requested loop |
| `span_percent` | 20–100 | Dynamic only: total travel around the midpoint; the floor avoids stall-prone micro-motion |
| `anchors` | 2–6 of `base`, `lower`, `middle`, `upper`, `tip` | Dynamic only: ordered semantic route; consecutive duplicates and routes narrower than 20% are rejected |
| `variation_percent` | 0–100 | Dynamic only: bounded deterministic drift over several cycles, never high-frequency noise |
| `segment_seconds` | 4–120 | Dynamic only: model decision horizon, not a stop timer |

Validation enforces the mode-specific combinations. A Dynamic start requires
speed and either center plus span or anchors. A Dynamic update may omit fields
to preserve their live values but must change at least one value. A Pattern
Library start or target cannot include Dynamic fields. `none` and `stop` carry
no target fields in either mode. The decoder still accepts the retired
`intensity` alias from old saved responses, immediately normalizes it to
`speed_percent`, and never advertises it to a model. When both fields are
present, `speed_percent` wins. A stopped engine accepts only `start`; neither
`target` nor `update` starts motion as a side effect. A running Pattern Library
shape change may omit pace, and a Dynamic update may omit any unchanged field.
Disabled or unknown pattern ids are rejected. Dynamic never reads pattern
storage or receives catalog names.

Pattern choices describe shape and relative rhythm only. The prompt strips
storage/status tags (`experimental`, `curated`, `imported`) and never exposes
persisted IDs whose historical names imply a pace. The engine independently
normalizes each loop's total travel to the requested speed, subject to the
configured maximum and curve-specific acceleration/reversal safety floors.

After parsing, deterministic current-turn authorization strips `start` or a
Pattern Library `target` unless the current user message contains a positive,
action-specific motion request in one of the supported prompt languages.
Dynamic `update` deliberately has broader admission while Dynamic motion is
already running: any non-empty, non-negated current turn may carry the model's
choice to update or not update. This keeps semantic taste and non-deterministic
action/no-action ownership with the model instead of reconstructing the legacy
phrase ruleset. Negation still blocks an update, and Dynamic `start` still
requires explicit current-turn authority.
An unauthorized command returns as inert reply text before semantic repair, so
fallback cannot recreate it. Autopilot is the sole exception to the user-turn
matcher: its decision message is generated inside the mode manager and carries
that existing autonomous-mode authority, while still passing capability,
semantic, enabled-pattern, mode-lifecycle, engine, and transport gates.
Authorization never widens capabilities: an allowed command still passes every
existing combination, state, speed-band, engine, and transport check. `stop`
remains unconditionally safe, and conservative exact Chat Stop phrases bypass
the model in every built-in prompt language.

A standalone embodied partner-action request such as `fuck me`, `suck me`,
`kiss it`, `stroke me`, or `ride me` can grant current-turn permission for a
`start` from stopped or a `target` from running even when it does not mention
the device. That permission is not a
deterministic instruction to move: the model decides from the current wording
and conversation whether to return `start` or no motion. The backend never
synthesizes an omitted start. Ordinary chat retains stochastic model sampling
(`temperature=0.3`, `top_p=0.95`), so repeated equivalent turns can produce
different model-owned action/no-action choices; there is no backend coin flip
or phrase-to-action ruleset. The matcher's only role is to prevent an otherwise
valid model command from exceeding the user's current-turn authority; negated
requests, quoted wording, definitions, stories, and conversational expletives
such as `well, fuck me` remain inert. A model-selected semantic start still
passes normal engine admission, target normalization, configured speed/range
limits, Stop epochs, and the selected transport; it is not a private motion
path.

Each interactive turn also receives one authoritative runtime snapshot:
stopped/running/paused state, current speed, and the persisted speed envelope
split into low/middle/high bands. Pattern Library adds the active pattern/area
and up to four recent chat-selected opaque handles. Dynamic instead adds the
live center, span, ordered anchors, variation, and decision horizon. This state
is prompt data, not a second frontend motion model. It is derived from the
engine snapshot and bounded trace ring, so it is runtime-only and requires no
database migration.

Opted-in interactive non-utility chat receives a separate backend-owned conversation
snapshot: bounded persona/anatomy settings, the effective session mood, and the
latest three canonical assistant lines (one line and 180 Unicode characters
each). User-authored profile values and prior replies are JSON-quoted as data.
This snapshot never enters the user message or semantic motion validator. Mood
is stored in the existing `diagnostics_json`, so it also needs no schema
migration. The broader 12-message history is likewise rebuilt from the selected
server-side session rather than trusted from the request. Utility prompts remain
profile-free, do not update mood, and suppress its readout; Autopilot motion
decisions deliberately exclude profile context so quoted persona data cannot
steer an autonomous motion decision.

Continuity and variation are separate intents. In Pattern Library mode,
ordinary conversation, "continue", and steady/hold requests preserve motion;
pacing-only requests preserve content and area. In Dynamic mode, omitted fields
preserve live geometry and a valid `none` remains authoritative even for a
variation request. The backend does not force a random change merely to differ.

Semantic no-op targets receive one repair pass. If the model repeats a no-op,
deterministic recovery drops the motion command and preserves the valid reply;
it does not synthesize a pattern change. Authorization, capability gates,
enabled-content checks, configured limits, and the shared motion engine remain
deterministic. Content-selection taste and conversational variation stay with
the model instead of a second hard-coded motion policy.

Chat Autopilot reuses the selected contract at bounded decision boundaries. In
Pattern Library mode it may curate an enabled pattern/speed or hold. In Dynamic
mode it may start or update geometry, and its `segment_seconds` is clamped to
the user's Autopilot motion-cadence range. Dynamic provider failure holds the
current Dynamic target or waits for a model decision; it never falls back to a
deterministic library pattern. An accepted interactive chat target temporarily
suspends decision dispatch and then becomes Autopilot's current segment. A
generation token prevents late adoption after Stop or a mode change. This is
orchestration of the shared engine, not a second motion loop.

## What the engine already supports that the model cannot reach

`internal/motion/target.go` — `MotionTarget`, the app-level semantic intent the
engine actually consumes — is richer than the chat contract that feeds it:

| Engine field | Capability | Reachable from chat today? |
| --- | --- | --- |
| `PatternID` | repeatable pattern | **yes** |
| `SpeedPercent` | speed within limits | **yes** |
| `AreaFocus{MinPercent,MaxPercent}` | constrain sampling to a **stroke region** | **yes**, through named zones |
| `Dynamic` | ephemeral center/span or named-anchor loop with slow variation | **yes**, in Dynamic mode |
| `SoftAnchor{PositionPercent,WeightPercent}` | gently bias motion toward a point | **no** |
| `ProgramID` | play a finite **program/funscript** | **no** |

Soft anchors and program playback already exist in the engine and its tests but
remain outside the model contract. The near-term ideas below are mostly about
*safely exposing existing engine capability through a versioned contract*, not
about building new motion.

## Capability gates and live-provider evidence

The sidebar and Settings > Model expose the persisted Dynamic / Pattern Library
/ Off mode list. Pattern mode additionally exposes area focus and experimental
patterns. Disabled methods are absent from the prompt and stripped from model
noise before dispatch. The setting lives in the existing versioned settings
JSON document in SQLite; an older document without the field preserves Pattern
Library behavior, except an existing chat-only capability maps to Off. No
schema/table migration is needed.

The 2026-07-20 live matrix exercised the final service against both supported
provider paths with a 20–40% test envelope and no transport dispatch:

- managed `llama.cpp b9966` CUDA with the installed Gemma 4 11.9B Q4_0 model:
  13/13 first-pass valid turns; start 23%, relative increase 33%, pattern
  targets 30%; hold, area clear, and chat-only behavior were correct; five
  repeated variation requests selected five distinct patterns before an older
  choice became eligible
- Ollama with Granite 4.1 3B Q4_K_M as a deliberately weaker small-model case:
  all scenarios completed within the contract, with repair used where needed;
  the same five-turn variation sequence avoided immediate reuse and every speed
  stayed at or below 40%

This closes the Pattern Library interactive provider/prompt evidence. Dynamic
currently has parser, prompt-isolation, shared-engine, phase/velocity retarget,
Autopilot, trace, and frontend coverage. It does not yet have a managed
llama.cpp/Ollama model matrix or a matched real-device feel comparison, so it is
selectable but not the default.

## Ideas, ranked by leverage-to-risk

Each idea notes whether it **restores** reference-app parity or is **net-new**,
its main dependency, and an honest disposition. Ordering is a suggestion, not a
commitment.

### A. Stroke-region focus (parity; low risk) — **shipped 2026-07-20**

Implemented: the chat contract accepts `"area":"tip"|"shaft"|"base"|"full"` on
start/target; named zones localize to bounded windows in deterministic code
(tip 66–100, shaft 33–67, base 0–34; `full` clears). Focus persists through
ordinary chat and pacing-only adjustments. Broad variation may change or clear
it; a named focus is temporary unless the user explicitly asks to stay. A
focused loop contracts its cycle with the narrowed travel so it does not also
become physically slow, bounded by the source pattern's authored acceleration
budget. Region changes ride the engine's normal retarget path, and Autopilot
decisions carry the same field. Gated by the **Model motion control** checkbox
list in Settings > Model
(`llm.motion_capabilities`: motion / patterns / area focus / experimental
patterns) — disabled methods are never described to the model and are stripped
if emitted, without failing the turn. Live-verified against a local Ollama 3B:
`{"action":"target","area":"tip","speed_percent":25}` for "focus on the tip,
keep it gentle".

Revised 2026-07-24 after a report that area requests were unreliable and too
subtle. Three changes, measured in
[the motion pathway review](motion-pathway-review-2026-07-20.md#follow-up-review---2026-07-24):

- A zone request is authorized by a zone name plus a placement word, not by the
  single literal phrase `focus on the tip` behind a directive prefix. That
  branch reaches only `target`; starting motion is unchanged.
- A pattern confined to a zone re-expands its own span to fill it, so a
  narrow-amplitude pattern is not squashed twice.
- Named zones are semantic target state rather than a persistent user setting.
  `full` clears area focus, and requests narrower than the measured 20-point
  minimum are widened automatically.

### B. Program / script selection (parity; low–moderate risk)

Extend curation so the model can pick an enabled **program** (funscript), not
only a pattern — the reference app's historical item #16 `{script_id, intensity}` shape.
The engine already accepts `ProgramID`; the library already separates finite
programs from loops ([pattern-library.md](pattern-library.md)).

- Dependency: expose enabled program ids to the model as data (same
  enabled-only, curation-gated rule as patterns) and add a `program_id` branch
  to the contract with the same `speed_percent` field used by patterns.
- Disposition: good, but respect the reference app's own caution — with a small
  catalog "the LLM keeps picking the same two scripts" becomes the failure
  mode. Worth doing; pair with a note that catalog size gates its value.

### C. Bounded relative deltas (net-new polish; low risk) — **partially shipped**

Map "a little faster", "deeper", "shorter strokes" to bounded *relative*
adjustments of the current target, clamped **once** at the transport boundary.
The reference sweeps flag a real bug here: clamping speed at multiple layers
compounded. Keep it single-clamp and visible.

- Current state: the authoritative snapshot lets the model translate phrases
  such as "a little faster" into a nearby absolute semantic speed while
  preserving content and area. Deterministic speed-band prompt tests and both
  live providers cover that path. Raw model-authored delta fields and
  stroke-depth deltas are still intentionally absent.

### D. Style / mood bias as visible state (parity; low risk) — **partially shipped**

The model already biases Freestyle indirectly; make the **Motion Style**
(gentle/balanced/intense) a visible, model-readable, model-settable field rather
than hidden prompt drift, surfaced in diagnostics. Reference ROADMAP #4 reached
the same conclusion ("steer model behavior without hidden prompt drift").

- Current state: model-reported reply mood is now a strict, visible,
  backend-authoritative per-session state. It is deliberately inert metadata,
  not inferred sentiment and not a motion-style shortcut.
- Remaining disposition: making **Motion Style** model-settable remains a
  separate idea. It must be an explicit semantic field consumed by deterministic
  scoring, never hidden prompt drift or an interpretation of `new_mood`.

### E. Dynamic model-authored motion (reference direction; moderate risk) — **shipped, acceptance open**

Dynamic implements the useful part of the reference app's default non-library
control without copying its private timers, UI state, or transport writes. The
model requests bounded center/span geometry or an ordered route through named
anchors, pace, slow variation, and a 4–120 second decision horizon. The backend
compiles that request into ordinary sampled content and `MotionPlan`; the shared
engine still owns startup, retargeting, limits, transport framing, and Stop.

Interior anchor knots carry a non-zero tangent so they are pass-through points,
not accidental stops. True route endpoints reverse. Variation is deterministic,
bounded, loop-closed drift over several cycles; it cannot add high-frequency
shake. Active retarget phase selection scores position, direction, and bounded
velocity mismatch, and the existing C1 handoff remains the only bridge. A
horizon-only update preserves content identity and phase.

The remaining acceptance gate is empirical: compare Dynamic and Pattern Library
on the same local models and real device at capped speed, including slow narrow
motion, route reversals, repeated conversational updates, Autopilot handoffs,
and Stop. Do not make Dynamic the default from simulation alone.

### F. Chat Autopilot session controls (partially implemented; moderate)

The initial explicit session control now lives on Chat, not Preset Modes. While
enabled, the model curates authored pattern content at segment boundaries from
bounded conversation context; the shared mode manager remains only the backend
execution/lifecycle owner. This placement keeps assistant autonomy with the
conversation and leaves Freestyle as the clearly separate deterministic preset
behavior.

The cadence and speech-authority portion is specified in
[autopilot-cadence.md](autopilot-cadence.md): motion evolution and speech use
independent clocks, model timing is categorical and code-bounded, and a hold is
scheduler-only. The sidebar now provides the visible session-level choice
between curated authored content and Dynamic geometry. Off prevents Autopilot
start. A guarded `mode_action` field is still needed before the model itself may
enter or leave Autopilot, especially from voice transcripts. Model-triggered
mode changes remain off until that explicit opt-in exists.

### G. Soft-anchor waypoints (parity; moderate)

The Dynamic contract now accepts an ordered loop through 2–6 named anchors
(tip/upper/middle/lower/base), and the engine compiles interior anchors as
pass-through knots. This is not the existing weighted `SoftAnchor` field: it is
ephemeral route geometry and cannot be persisted or selected as a preset.

- Remaining disposition: saved, visible weighted-anchor presets are still
  attractive after Dynamic A/B acceptance. Prefer the model selecting an
  inspectable preset over exposing raw `SoftAnchor` weights.

## New ideas not in the reference notes

These are not in the StrokeGPT-ReVibed docs; offered for consideration with the
same skepticism.

- **Suggest-then-confirm previews.** For a large change (new arrangement, new
  program), the model proposes and the UI shows a one-line summary the user
  accepts before it drives the device. Reuses the backend preview sampler the
  library already renders. Turns "the model surprised me" into an approval step
  without adding a stop.
- **Named user presets as first-class model choices.** Let users save an
  arrangement/region/anchor set as a named preset; the model then selects a
  *preset id* the same way it selects a pattern id. Keeps the model's surface
  small and inspectable and lets users curate what it may reach.
- **Typed/spoken control parity.** Whatever contract fields exist must behave
  identically whether the turn arrived by keyboard or by ASR transcript — with
  the reference app's caveat that model-selected *mode changes* from voice stay
  behind their own explicit gate.
- **Trace-visible "what changed."** Every model-driven change (region, pattern,
  program, style, mode, speed) should be legible in diagnostics/trace, so a user
  can always answer "why did it do that?" This is a documentation/observability
  requirement on all of the above, not a feature.
- **Sentiment-paced drift — flagged, not recommended.** Tie speed drift to
  conversational escalation. Tempting, but it is hidden state and easy to
  overfit; the reference app reached the same verdict on fuzzy controllers
  ("likely too noisy without large-scale human input; treat as a research
  spike"). Record it here so it is not re-proposed as a quick win.

## Cross-references

- Current contract: `internal/chat/contract.go`; engine intent:
  `internal/motion/target.go`; system-prompt assembly: `internal/chat/prompts.go`.
- [ADR 0015](decisions/0015-selectable-llm-motion-modes.md) — the persisted
  Dynamic / Pattern Library / Off decision and acceptance gate.
- [motion-retargeting.md](motion-retargeting.md) — the sampler/retarget model
  any new intent compiles through.
- [pattern-library.md](pattern-library.md) — patterns vs programs, enabled-only
  curation, and the LLM catalog rule.
- [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md) — the initial Chat
  Autopilot slice and the still-open arrangement/session controls (ideas E/F).
- Reference: `StrokeGPT-ReVibed/docs/motion_control_modes.md` (route policy and
  "Future LLM Control Modes") and `ROADMAP.md` items #3, #4, #7, #12, #15, #16.
