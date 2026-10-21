# Device-aware model responses plan

Status: plan for later work, written before runtime changes (2026-10-15). Not
scheduled. Base: the device stack in PRs #319 and #321 and the
[Intiface vibrator plan](intiface-vibrator-plan-2026-10-15.md). Nothing here
changes a prompt yet.

## Problem

Model replies and motion choices assume a linear stroker. That is correct for
The Handy and for a Buttplug linear actuator, but not when the motion frame is
felt only through a companion conversion: Coyote-only playback (shipping in
#319) and vibrator-only playback (planned). The owner flagged this as the
reason non-Handy devices need prompt work before they are first-class.

Where the assumption lives today:

- The motion contract describes positions along a stroke: zones are "tip",
  "shaft" and "base"; `center_percent` is the midpoint of travel from base/deep
  (0) to tip/shallow (100); `span_percent` is stroke length; anchors are
  base/lower/middle/upper/tip; full-length wording must cover base and tip
  (`internal/chat/prompts.go`, `contractAreaSection` and the motion contract).
- Stroke, reach and travel wording also runs through Creative v2's reach
  guide, the Layered and dynamic-intent prompts, the Autopilot prompts and
  facts (`internal/chat/autopilot*.go`) and the stroke and LLM labs.
- The video authority note names "pace, depth, rhythm or intensity".
- 14 of the 27 built-in pattern descriptions mention strokes, depth or length
  (`internal/motion/content.go`); the rest describe windows, reach and
  reversals.
- Reply guidance tells the persona to talk about "what you are doing to me";
  in practice that is stroking.
- The live evaluations fix one device picture (for example
  `UserAnatomy: "penis"` in `internal/chat/live_prompt_eval_test.go`).

The Coyote companion deliberately added no prompt text
([llm-control-surface.md](llm-control-surface.md)): the model has no strength
control, and with a Handy present its stroke vocabulary is still true.

## Goal

For each device configuration, replies and motion choices are truthful about
what the person feels: the model knows which of its motion choices are felt and
how, never claims control it does not have, and never invents device details.
One JSON motion contract and one shared engine remain.

## Principles

- **One contract.** The motion JSON schema does not change per device. Only
  the field descriptions, examples, offered capabilities and reply guidance
  change.
- **Backend facts only.** The device section is composed from the playback
  participants and each companion's conversion profile in the backend snapshot
  at turn time. It is never inferred from chat text, and no keyword or phrase
  rule filters replies.
- **Offer what is rendered.** A capability is offered only when some
  participant renders it. Zone focus and anchors need position rendering: a
  stroker, the Coyote's stroke or position profile, or the vibrator's depth
  profile. Pace is rendered by every configuration.
- **Examples steer.** Prompt examples strongly bias the model's choices
  ([Creative v2 roaming review](creative-v2-roaming-review-2026-09-05.md)), so
  each configuration gets its own examples rather than a caveat sentence, and
  they are reviewed for the same bias.
- **No implied strength control.** The model cannot change Coyote strength,
  the vibrator's live intensity or any ceiling. A reply may say a sensation
  follows the pace when the profile makes that true; it never claims to turn
  a level up.
- **Anatomy stays explicit.** The user's anatomy setting is never inferred
  from device kinds.
- **The Handy-only prompt stays unchanged** unless a measured improvement
  justifies it; its composition is golden-tested byte for byte.
- **Local and hosted prompts may diverge**, each measured on its own.
- **Localization.** Reply-shaping text ships in all five prompt locales, like
  voice identity and anatomy (`internal/chat/prompt_localization.go`); the
  motion contract stays English.

## Proposed changes per configuration

| Configuration | What the frame becomes | Contract changes | Reply guidance |
| --- | --- | --- | --- |
| The Handy, or an Intiface linear device | strokes | none | none; a non-Handy device is named neutrally |
| Stroker + Coyote | strokes; e-stim follows | none | short companion note: an e-stim unit follows the motion and the user sets its strength |
| Stroker + vibrator | strokes; vibration follows | none | companion note per vibrator profile (follows pace, strongest near one end, or steady) |
| Coyote only | pulse rate, width and channel balance from the frame, per profile | stroke wording replaced; zones/anchors offered only for profiles that render position | rhythm and sensation; no strokes, depth or reach |
| Vibrator only | vibration level from pace, position or steady | follow-speed and steady: no zones or anchors; follow-depth: zones described as intensity regions | vibration intensity and rhythm; no strokes or depth |
| Coyote + vibrator | both follow; no linear motion | union of what the two profiles render | rhythm and sensation |
| Chat only | no device | existing chat-only text | existing |

The same participant facts feed every model-facing surface: chat, Autopilot
check-ins and continuity, Freestyle and dynamic intent, the LLM lab, and
pattern descriptions in Pattern mode.

## Testing per device configuration

Configuration matrix, in priority order:

1. The Handy only (baseline; must not regress)
2. Vibrator only, follow speed
3. Vibrator only, follow depth
4. Coyote only, stroke-and-speed profile
5. Coyote only, speed-only profile
6. Handy + vibrator
7. Handy + Coyote
8. Coyote + vibrator
9. Handy + Coyote + vibrator
10. Intiface linear device (neutral naming)
11. Chat only (no devices)

Deterministic tests (CI, no model):

- composition per configuration: which sections and capabilities appear,
  which examples are used, and that nothing outside the participant set is
  mentioned
- a byte-for-byte golden for the Handy-only composition
- locale coverage for every new reply-shaping string, and the prompt budget
  tests with the device section included
- the prompt inspector shows the device section

Live evaluations (opt-in, against a real model):

- Add a device-configuration dimension to the `liveeval` Go tests
  (`MAGICHANDY_LIVE_LLAMA_URL`) and to the app evaluators
  (`scripts/evaluate-app-conversation.py`, `-controls`, `-focus`,
  `-mode-switch`, `-autopilot`), using simulated outputs so no hardware is
  involved: `-simulate-motion` with the simulated Coyote and a simulated
  Intiface vibrator.
- Record per configuration: JSON validity, repair and fallback rates,
  agreement between reply and motion, and response time.
- A reviewed rubric for each sampled reply: describes sensations the active
  devices can produce; makes no claim of strength, ceiling or device control;
  invents no device details; keeps the persona's voice; respects the anatomy
  setting. An LLM judge may pre-sort samples; a person makes the final call.
- Run the local default model first (small models are the sensitive case),
  then hosted connections. The Handy-only results must stay within the current
  baseline's noise.

Physical acceptance: the owner runs short sessions in each configuration they
own and rates whether replies match what they feel.

## Order of work

1. Plumb participant facts into prompt composition (`Capabilities`), show them
   in the prompt inspector, and add the deterministic tests. No prompt text
   changes.
2. Companion notes for stroker + Coyote and stroker + vibrator.
3. Solo non-linear vocabularies (vibrator only, Coyote only, Coyote +
   vibrator) with per-configuration examples.
4. Device-aware pattern descriptions for Pattern mode.

Each step lands only after the live-evaluation matrix for the configurations
it touches.

## Until this lands

Vibrator-only and Coyote-only playback use today's prompts, so replies can
describe strokes or depth that the person does not feel. The motion itself is
correct: the shared frame drives each companion through its profile. Whether
the connection manager should say so in one line is a UI decision for the
owner.

## Out of scope

- LLM control of channel strength, the vibrator maximum or any ceiling.
- Device-specific JSON fields or a second motion contract.
- Native stimulation patterns or a stimulation vocabulary (recorded follow-up
  of [ADR 0044](decisions/0044-coyote-estim-output.md)).

## References

- [llm-control-surface.md](llm-control-surface.md)
- [ADR 0044](decisions/0044-coyote-estim-output.md),
  [ADR 0045](decisions/0045-device-inventory-and-linked-sessions.md)
- [Intiface vibrator plan](intiface-vibrator-plan-2026-10-15.md)
- `internal/chat/prompts.go`, `internal/chat/prompt_localization.go`,
  `internal/chat/live_prompt_eval_test.go`, `scripts/evaluate-app-*.py`
