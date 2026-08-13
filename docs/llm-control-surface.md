# LLM Motion Control Surface — Current State and Ideas

This document catalogs how the local LLM can drive the device today, what the
motion engine can already do that the model cannot yet reach, and a ranked set
of ideas for widening LLM control. The initial Chat Autopilot slice landed in
PR #101; the remaining ideas are design inputs, not implied commitments.

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

So the model emits **semantic intent** (a region, an opaque pattern handle, a speed, a
named arrangement). Deterministic code compiles that into motion and clamps it
to the user's speed/stroke/limit envelope. Speed and stroke limits stay
transport-layer caps, never prompt-only behavior. Emergency Stop stays
independent of every generation, upload, and playback latency.

## What the LLM can emit today

`internal/chat/contract.go` accepts exactly one `AssistantResponse`: required
user-facing `reply`, optional semantic `motion`, and optional `new_mood` for
interactive non-utility chat. `new_mood` is a strict 17-value reply-register
enum. It is persisted as session diagnostics and shown as backend-reported Chat
state, but it has no representation in `MotionCommand`, `MotionContext`,
`MotionTarget`, or transport dispatch.

`MotionCommand`, the only model-authored motion shape the parser accepts:

| Field | Values | Meaning |
| --- | --- | --- |
| `action` | `none` / `start` / `target` / `stop` | start, retarget, or stop motion through the engine |
| `pattern_id` | an opaque **enabled** catalog handle | curate one enabled shape; parsing resolves it to a stable internal ID |
| `speed_percent` | 1–100 | semantic playback speed, clamped again by the user's limits |
| `area` | `tip` / `shaft` / `base` / `full` | select a named stroke zone; `full` clears area focus |

Validation enforces the safe combinations: a pattern requires speed, and
`none`/`stop` carry no target fields. The decoder still accepts the retired
`intensity` alias from old saved responses, immediately normalizes it to
`speed_percent`, and never advertises it to a model. When both fields are
present, `speed_percent` wins. A stopped engine accepts only `start`; `target` never starts motion as
a side effect. A running pattern change may omit pace, in which case
deterministic code preserves the current speed. Disabled or unknown pattern ids
are rejected, and an all-disabled library keeps the deterministic speed-only
contract. This is real curation — the model selects from author-owned content —
but it is still narrower than the engine.

Pattern choices describe shape and relative rhythm only. The prompt strips
storage/status tags (`experimental`, `curated`, `imported`) and never exposes
persisted IDs whose historical names imply a pace. The engine independently
normalizes each loop's total travel to the requested speed, subject to the
configured maximum and curve-specific acceleration/reversal safety floors.

After parsing, deterministic current-turn authorization strips `start` or
`target` unless the current user message contains a positive, action-specific
motion request in one of the supported prompt languages. Negation and common
conversation objects are rejected before matching; broad standalone topic
words such as `different`, `continue`, or `pattern` do not authorize motion.
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
stopped/running/paused state, current pattern or program, current speed and
area, the persisted speed envelope split into low/middle/high bands, and up to
four recent chat-selected patterns. Pattern references use the same
deterministic opaque handles as the enabled catalog, so legacy pace-biased
database IDs cannot steer selection. This state is prompt data, not a second
frontend motion model. It is derived from the engine snapshot and bounded trace
ring, so it is deliberately runtime-only and requires no database migration.

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

Continuity and variation are separate intents. Ordinary conversation,
"continue", and steady/hold requests preserve motion; pacing-only requests
preserve content and area. For an explicit variation request, the model owns
the semantic choice across pattern, speed, and area. Any meaningful target
change is valid, including an area-only move back to `full` or a speed change
that keeps the current pattern. Recent pattern handles are prompt context rather
than a deterministic exclusion list.

Semantic no-op targets receive one repair pass. If the model repeats a no-op,
deterministic recovery drops the motion command and preserves the valid reply;
it does not synthesize a pattern change. Authorization, capability gates,
enabled-content checks, configured limits, and the shared motion engine remain
deterministic. Content-selection taste and conversational variation stay with
the model instead of a second hard-coded motion policy.

Chat Autopilot reuses this same contract at bounded segment boundaries. Its
request includes the latest 12 canonical conversation messages, current style
and speed band, the engine's live pattern, speed, and area, recent pattern handles,
and the last autonomous line. It may curate an enabled pattern/speed or
hold; deterministic code owns duration and all clamps. An accepted interactive
chat target temporarily suspends decision dispatch, then becomes Autopilot's
current segment after the engine applies it; a stale in-flight decision cannot
restore the previous focus. A generation token prevents late adoption after
Stop or a mode change. This is broader orchestration of the existing contract,
not a second motion schema.

## What the engine already supports that the model cannot reach

`internal/motion/target.go` — `MotionTarget`, the app-level semantic intent the
engine actually consumes — is richer than the chat contract that feeds it:

| Engine field | Capability | Reachable from chat today? |
| --- | --- | --- |
| `PatternID` | repeatable pattern | **yes** |
| `SpeedPercent` | speed within limits | **yes** |
| `AreaFocus{MinPercent,MaxPercent}` | constrain sampling to a **stroke region** | **yes**, through named zones |
| `SoftAnchor{PositionPercent,WeightPercent}` | gently bias motion toward a point | **no** |
| `ProgramID` | play a finite **program/funscript** | **no** |

Soft anchors and program playback already exist in the engine and its tests but
remain outside the model contract. The near-term ideas below are mostly about
*safely exposing existing engine capability through a versioned contract*, not
about building new motion.

## Capability gates and live-provider evidence

Settings > Model exposes four persisted model permissions: motion commands,
pattern selection, area focus, and experimental patterns. Dependencies remain
visible but disabled when their parent permission is off. Disabled methods are
absent from the prompt and stripped from model noise before dispatch. The
settings live in the existing versioned settings JSON document in SQLite;
absent older values resolve to conservative defaults, so no schema/table
migration is needed.

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

This closes interactive provider/prompt evidence. It is not a real-device feel
check or a long-running Chat Autopilot acceptance run; those remain separate.

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

### E. LLM-requested motion arrangement (net-new; moderate risk)

Let the model *request* a bounded arrangement — named styles, focus regions,
durations, repetitions, speed drift — that compiles through the existing
Phase 11 arrangement contract, instead of nudging low-level targets every turn.
This is precisely the reference app's "Preferred direction" in
`motion_control_modes.md` and its item #16 arrangement note. MagicHandy already
has the compiler (Freestyle uses it); the new part is a safe *request* schema.

- Dependency: a versioned `arrangement` request shape plus validation that it
  stays inside the 1–8 segment / 4–120 s bounds the planner already enforces.
- Disposition: high-value and architecturally aligned, but it is the first idea
  that grows the schema meaningfully — version it, keep each segment long enough
  to establish feel (multiple cycles), and surface the active arrangement in
  diagnostics so users can see what the model changed.

### F. Chat Autopilot session controls (partially implemented; moderate)

The initial explicit session control now lives on Chat, not Preset Modes. While
enabled, the model curates authored pattern content at segment boundaries from
bounded conversation context; the shared mode manager remains only the backend
execution/lifecycle owner. This placement keeps assistant autonomy with the
conversation and leaves Freestyle as the clearly separate deterministic preset
behavior.

The cadence and speech-authority portion is now specified in
[autopilot-cadence.md](autopilot-cadence.md): motion evolution and speech use
independent clocks, model timing is categorical and code-bounded, and a hold is
scheduler-only. Remaining work is a visible **session-level autonomy choice**
between curated authored content (`{pattern_id or program_id, speed_percent}`) and
future freeform arrangements. A
guarded `mode_action` field is still needed before the model itself may enter or
leave a session, especially from voice transcripts. Model-triggered mode
changes remain off until that explicit opt-in exists.

### G. Soft-anchor waypoints (parity; moderate)

Expose the engine's `SoftAnchor` so the model (or a saved preset) can bias
motion toward 2–6 waypoints (tip/upper/middle/lower/base) that motion slides
through without hard stops. Reference ROADMAP #7 designed exactly this as an
inspectable control surface.

- Disposition: attractive but do it after A/B; the reference app deliberately
  put visible *authoring* of soft anchors ahead of letting the model invent
  them. Prefer "model selects a saved soft-anchor preset" over "model emits raw
  waypoints."

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
- [motion-retargeting.md](motion-retargeting.md) — the sampler/retarget model
  any new intent compiles through.
- [pattern-library.md](pattern-library.md) — patterns vs programs, enabled-only
  curation, and the LLM catalog rule.
- [IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md) — the initial Chat
  Autopilot slice and the still-open arrangement/session controls (ideas E/F).
- Reference: `StrokeGPT-ReVibed/docs/motion_control_modes.md` (route policy and
  "Future LLM Control Modes") and `ROADMAP.md` items #3, #4, #7, #12, #15, #16.
