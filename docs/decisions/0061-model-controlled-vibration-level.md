# ADR 0061: Model-controlled vibration level under a user max

Status: implemented with automated, simulated and live-model acceptance,
2026-10-28. Physical vibrator acceptance remains open.

> Amended by [ADR 0063](0063-perceptible-following-and-coyote-facts.md):
> following speed reads each stroke over one 100 ms tick, with 40/100 ms
> smoothing, and the band floor is 20% rather than the 35% of decision 7. The
> first physical test found the earlier speed following close to constant.

This supersedes the intensity authority in [ADR 0047](0047-intiface-vibrator-output.md)
("the model never controls its intensity") and the Stop restoration rules in
[ADR 0058](0058-model-capability-levels.md) and
[ADR 0059](0059-manual-chat-resume-after-stop.md). The connection, pacing,
quantization, watchdog and Stop protocol of ADR 0047 are unchanged, as is the
shared-engine envelope of [ADR 0057](0057-independent-vibration-envelope.md)
apart from its unit.

## Context

The owner asked for a vibrator whose intensity the model controls. The alpha.76
review (2026-10-27) found:

- The live intensity was never saved. It returned to 0 when the device was
  chosen, released or dropped, when Intiface connected or failed, and at Stop,
  so a session started silent until a person found the panel's + button.
  Advanced models shaped drive only inside that number; others could not
  change vibration at all, and Simple models were not told a vibrator existed.
- Strength appeared as three numbers on two surfaces: the panel's live
  intensity ("20% of 40", the 40 an unlabeled ceiling), the Settings page's
  minimum-to-ceiling range, and the model's relative envelope. None was the
  motor level, and the minimum was ignored by independent envelopes.
- Following speed barely followed. Through the shared planner and the real
  renderer, the default 20–80% speed band moved drive only from 0.56 to 0.92:
  22% to 35% at the default 40% ceiling, three or four motor steps.

The capability levels target specific models: Simple Gemma E4B, Intermediate
Gemma 4 12B and Advanced GPT 6 Sol. The 12B managed the four-field independent
envelope in only 4 of 20 turns; GPT managed it in 24 of 24.

## Decision

1. **One user-owned Max.** The saved ceiling is called Max. It caps every
   output, including Test, and appears as the same setting in the device panel
   and on the vibrator's page. The minimum becomes "Minimum while moving": the
   floor of ordinary movement-following vibration, at or below Max.
2. **Model control is a saved per-device choice, on by default**
   (`vibration.level_control`: `model` or `manual`). It is effective only when
   the chat model has a vibration vocabulary; otherwise, and with it off,
   vibration follows movement at Max and no model receives a vibration field.
3. **The model chooses its starting level.** Until it chooses, speed-following
   vibration follows the shared motion under Max; depth and steady profiles
   derive their level from the commanded Handy speed within the saved speed
   band. There is no fixed initial model level. Once chosen, the level lives
   on the vibrator owner and survives connecting, selection and Stop within
   this app process. A connected, selected vibrator is silent until playback.
4. **Vocabulary by capability level.**
   - Simple (Layered) and Intermediate (Layered and Creative v2) get one
     nullable integer, `vibration_level`: 0–100% of Max, null to keep. It is a
     live-chat field; Autopilot keeps the level chat chose. Vibration follows
     movement through the saved profile; 0 silences only the vibrator. The
     prompt states that a vibration-only request is action none with empty
     edits, and while the level is 0 the backend reports the last nonzero level
     (`previous_level_percent`) for "back on". Simple Creative v2 has no level
     and follows at Max: E4B overran its output there (see Evidence).
   - Advanced keeps ADR 0057's envelope, now in % of Max instead of relative
     to an armed intensity. `follow` may carry its own range (`min_percent`
     while moving, `max_percent` at its strongest); all zeros follow at the
     owner's level.
5. **Glide only where it helps.** A Simple or Intermediate level change glides
   at up to the full range per 1.5 s. Advanced changes, the user's Max and Stop
   apply at once; Advanced is never rate-limited. The old 600 ms independent
   envelope retarget ramp is removed as well. The startup fade at Play and
   Resume stays, and wave/pulse keep their authored rounded shape.
6. **Stop pauses model control.** Emergency Stop silences the device as before
   and suspends the vibrator. A fresh manual chat request that applies motion
   or independently starts vibration, **Resume now** in the device panel
   (`POST /api/transport/intiface/vibrator/resume`, host and current Stop
   sequence), or choosing the device again lifts it. Settings changes,
   Autopilot and reviewers do not; a model raise is refused while it stands.
   A harmless level/follow echo is still accepted, but the suspension is
   restored if dispatch finishes without playback. This uses the execution
   result, not a classifier of the user's language or stricter reply grammar.
   The last continuous score remains available as stopped context. Advanced
   models receive a separately named `previous_envelope` for continuation;
   neither grants background start authority.
7. **Following speed spans the user's speed band.** Averaged absolute velocity
   maps linearly from 35% of the level at the saved minimum Handy speed to the
   full level at the saved maximum, and falls to zero as the Handy stops. Depth
   and steady are unchanged.
8. **An echoed stroke width is not a change.** The prompt shows an unset widest
   stroke as the band's width. A reply that copies it back is the same
   geometry, so it no longer retargets a running plan.
9. **Equivalent pace expressions apply once.** Intermediate may name an
   absolute pace and a relative adjustment that resolve to exactly the same
   value against the current (or recalled) score. Those are one change;
   conflicting values, duplicate JSON keys and invalid deltas still fail.
10. **Coyote strength is unchanged.** ADR 0044's user-only strength stands at
   every capability level.

## Safety and limitations

The model never sets Max, model control, playback participation, a device,
individual motors or raw commands, and cannot lift a Stop suspension. Output
remains a function of the shared engine frame and quantizes down to the motor
step at or below Max, rechecked at write. All motors still move together.

Removing the per-session live intensity gives up "disarmed until a person
raises it" for vibrators the user has put in playback. Max (default 40%), the
playback switch, model control and the Stop suspension replace it. The default
Max is 40%; the initial output now depends on motion and the model's choice,
not an assumed 50% model level. Disabling model control immediately follows
movement at Max, including when an independent envelope was already active.
Re-enabling it makes the saved model pattern eligible again; Stop still gates
all output.

No real vibrator was connected. Physical acceptance remains open: perceived
speed coupling, glide feel and Stop-to-silence time.

## Evidence

Renderer measurement (real planner and sampler, 100 ms owner cadence, full
sweeps over the default 20–80% speed band, 20-step motor): drive moved from
0.56–0.92 to 0.38–0.96; at level 100% under Max 40% the motor ranges from
15.5% to 40% in seven distinct steps (alpha.76: 21.8% to 35%). Turnarounds keep
more shape (10th–90th percentile 0.23–0.49 at speed 20, was 0.38–0.68).

Live evaluation (`TestVibrationLevelLive`, liveeval tag): thirteen intents in
Layered and Creative v2 through the production prompt, schema and parser:
stronger, gentler, off with unchanged strokes, full, slower strokes with
stronger vibration, faster only, a question, a soft start, a little more, back
on, plain and strong starts, and stronger after a slow start (below). A pass needs the right level change, preserved linear state and no
repair. Rejected and wrong choices are retained in the captures.

| Model, level | Layered | Creative v2 |
| --- | ---: | ---: |
| Gemma E4B Q4_K_M, Simple, llama.cpp b11533, 16K, 256 output, two seeds | 24/26 | 8/20 in the earlier ten-intent trial (excluded) |
| Gemma 4 12B Q4_0, Intermediate, Ollama 32K/512, two seeds | 26/26 | 26/26 |
| GPT 6 Sol, Advanced, low reasoning, 64K | 13/13 | 13/13 |

The history intent is the review app's own conversation: a slow, gentle
start followed by "Make the vibration stronger." In the app the 12B raised the
level and also sped up the strokes while saying the pace was unchanged; with
that history it changed movement in 5 of 6 seeds under the first guide. The
explicit guide (vibration and movement are separate; vibration-only is action
none with empty edits; do not speed up strokes to strengthen vibration) made it
6 of 6. Its "back on" chose 0 or null in 3 of 4 runs, saying it restored the
"previous level"; reporting `previous_level_percent` made it 4 of 4. Its
remaining misses were a pace stated both absolutely and relatively. The final
parser accepts only mathematically equivalent pairs, so it keeps the requested
pace without applying the delta twice. Both final seeds pass all 26 cases.

E4B without any vocabulary (the alpha.76 contract, same intents) answered
vibration requests by changing the strokes. With the level it chose the right
direction and kept the strokes. Before the echo rule in decision 8, its Layered
vibration replies still sent an update with an unchanged score, which would
have retargeted the plan. In Creative v2 it copied the whole score and overran
256 tokens or needed repair, so Simple Creative v2 has no level. Its Layered
final misses are one soft start that exceeds 256 output tokens and one strong
start with conflicting geometry followed by an overlong repair. Expanded
guidance reduced acceptance to 20/26, and a 384-token budget gave 23/26, so
neither experiment ships. GPT kept
following the strokes when strengthening (`follow` up to 70%) and chose
`steady` for "as strong as I allow", soft starts and "back on".

End to end, the review app (simulated Intiface vibrator, Gemma 4 12B at
Intermediate) started slow and gentle at level 30% (motors 5% under Max 40%)
and moved to 50% (10%) on "stronger"; a text-only question changed nothing.

Validation: full Go tests and the full race suite before the final parser
follow-up, followed by passing final chat/HTTP API race checks. Windows security
blocked the final ordinary HTTP API executable and two child-process fixtures;
that ordinary full run is not green, and CI must establish it. Vet, including the liveeval and labs tags;
golangci-lint v2.12.2 (0 issues); the pure-Go Windows build; 1,069 frontend tests,
localization and type checks; the rebuilt embedded UI. Transport tests cover
the level, glide, follow range, Max at write, Stop suspension, Resume now and
manual resume rollback; HTTP tests cover dispatch, Stop, manual control and the
admission matrix. The available w64devkit compiler was used for race tests;
the shipping core remains CGO-free. The follow-up also covers failed-start
rollback after a level change, model control off during an existing envelope,
paused level edits, and harmless settings echoes after Stop. See the
[review and remaining physical checks](../vibrator-level-review-2026-10-28.md).
