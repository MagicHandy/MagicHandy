# LLM Lab stroke vocabularies — 2026-09-26

## Purpose

Layered edits persistent layers inside an outer band. The
[depth review](motion-depth-review-2026-09-25.md) found that Gemma reads its
anchors and stroke widths as absolute positions, so depth requests often stop
at the band's floor. Patching the contract again would keep that coupling.
This change instead adds three candidate replacements to the LLM Lab, where
they can be tested side by side before any of them reaches main chat.

The candidates keep what Layered does well: one persistent score, edits that
change only what the request names, and variation from seeded fields rather
than programmed sequences. They differ in what the model writes. All three
edit the same new score and compile through the same generator, so a
difference between them measures the vocabulary, not the motion engine.

## The stroke score

`motion.StrokeSpec` describes continuous motion by where every stroke turns,
in absolute slider positions:

- **Bottom and top turns.** The bottom turn is where a stroke bottoms out,
  0 being the base; the top turn is where it turns back toward the tip. Each
  turn has a usual position, a position it may go instead, and a character:
  steady, drift (wanders smoothly between the two), alternate (switches every
  two to six strokes), occasional (about one stroke in five goes to the other
  position) or roam (wanders in step with the other roaming turn, so the whole
  stroke moves). The usual top stays at least 10 above the usual bottom.
- **Accents.** Up to three distinct motifs appear at seeded chances per
  stroke: rarely 1/16, sometimes 1/8, often 1/4. Plunge takes one stroke to
  the base, full crosses the whole length, tip flicks and deep grinding work
  two to four short strokes at a turn, pause holds at the top, and slow draws
  out one stroke.
- **Timing variation**, 0–100, lets stroke timing breathe.

A loop holds 64 strokes. The joint leg fit shared with Creative v2 times
every leg inside the velocity, acceleration and jerk budgets, and the
ordinary plan, sampler and Stop play it. The score exists only in the Lab.
Production chat and Autopilot never produce it, and saved limits apply as for
any other score.

## Three vocabularies

- **Stroke ends** exposes the score's own fields: each turn's position,
  alternative and character, accents, speed and timing variation.
- **Groove and accents** has one groove, `bottom_at_percent`,
  `top_at_percent` and a feel (steady, breathing or wandering), plus accents
  and speed. The feel sets both turns' characters and the timing variation.
- **Plain words** has five depth words for where every stroke bottoms out
  (base, lower, middle, upper, tip = 0, 20, 40, 60, 80) and four pull-back
  words for where it turns back (tip, upper, middle, lower = 100, 80, 60, 40).
  It also has a variety word, accent words with one rate, and five pace steps
  inside the saved speed limits. Every combination is a valid stroke: a pull
  back at or below the depth turns back 20 points above it. A stroke that
  reaches the base or pulls back to the tip holds that end while it breathes.
  The current score is read back to the model as its nearest words.

All three parse strictly. An invalid or incomplete value rejects the whole
reply and nothing is repaired; omitted fields keep their values, and `{}`
changes nothing. Each prompt carries the shared depth frame. The modes start
from their own score, long strokes at about 20–80 at a medium pace. Switching
into or out of them starts a new Lab score. Lab Autopilot continues them with
the production continuous judgment of [ADR 0031](
decisions/0031-model-judged-autopilot-continuity.md).

## Streamlined LLM Lab

The Lab page was rebuilt around fast comparison:

- Buttons select the five main modes; More modes holds the other contracts.
- Live motion and Autopilot are switches that take effect at once. Stop, New
  chat, Configure and Help share their row, and changing the mode during a
  test restarts it in the new mode.
- Quick requests (Deeper, Not so deep, Just the tip, Whole length, Faster,
  Slower, Keep it like this, Surprise me) send a message in one tap.
- Each reply is a compact entry with its outcome and mode. Details shows the
  model, timing, calls, changed fields and raw output, and Send again repeats
  a message.
- Compare modes sends one message to each main mode through
  `POST /api/labs/llm/compare`, each from its own starting score. It shows
  every reply with its reach and a 12-second plotted estimate; Try this mode
  switches to it. The request records nothing, never plays or dispatches, and
  is refused while a reply is generating or a test runs. It has the same
  administrator admission and controller check as the other Lab calls.
- Motion now shows the plotted estimate, reach and effective pace, with the
  current score collapsed below.

## Measurements

### Setup

Gemma 4 12B (the heretic build in the installed app) ran on the app's
llama.cpp b9966 CUDA runtime over loopback, with one slot and a 32,768-token
context. Requests went through the Lab's own request and parser, with
schema-guided output and the Lab's temperature of 0.1. The nine chat cases of
`TestContinuousDepthLanguageLive` ran three times per mode, each from a
starting score confined to the case's region, with saved speed limits 6–31.
Creative v2 and Layered used the chat suite's starting scores; stroke scores
started with turns at the region's ends, drifting up to 10 points inward.

A turn **met** the depth when the compiled score passed the case's check,
which samples where strokes actually turn. **Right way** and **wrong way**
count moves of at least 5 points in the lower-turn median. A turn was about
0.8 seconds.

### Results

The first run used the original vocabularies:

| Mode | Met depth | Right way | Wrong way | Rejected |
| --- | ---: | ---: | ---: | ---: |
| Creative v2 | 23/27 | 27 | 0 | 0 |
| Layered | 9/27 | 18 | 0 | 9 |
| Stroke ends | 18/27 | 24 | 0 | 0 |
| Groove, first field names | 15/27 | 15 | 6 | 0 |
| Plain words, depth and length | 21/27 | 21 | 0 | 0 |

Two vocabulary flaws caused most of the new modes' misses:

- Groove first named its ends `deepest_percent` and `shallowest_percent`.
  Gemma read the number as an amount of depth: to go deeper it raised
  `deepest_percent` from 45 to 80 or 85, sending the strokes to the tip in all
  six "deeper" trials. Position names like Stroke ends' fixed this.
- Plain words first paired a depth word for where strokes sit with a length
  word. At full length the depth word did nothing, so "Just the tip" from a
  full-length score changed only the depth and nothing moved (6 of 6 tip
  trials). Naming each end made every word take effect alone.

Groove's and Stroke ends' accent descriptions also gained one sentence: accents
never move the usual strokes, so a change to every stroke edits the turns. The
second run used the revised stroke vocabularies:

| Mode | Met depth | Right way | Wrong way | Rejected |
| --- | ---: | ---: | ---: | ---: |
| Stroke ends | 18/27 | 24 | 0 | 0 |
| Groove and accents | 18/27 | 24 | 0 | 0 |
| Plain words | 27/27 | 27 | 0 | 0 |

Met depth per case, out of three, with Creative v2 and Layered from the first
run:

| Case | Creative v2 | Layered | Stroke ends | Groove | Plain words |
| --- | ---: | ---: | ---: | ---: | ---: |
| Deepthroat it | 3 | 0 | 3 | 3 | 3 |
| You're not going deep enough | 3 | 3 | 0 | 0 | 3 |
| Take it all the way down every time | 3 | 3 | 3 | 0 | 3 |
| Bottom out on every stroke | 3 | 3 | 3 | 3 | 3 |
| Slower and deeper | 2 | 0 | 0 | 0 | 3 |
| Just the tip for a while | 3 | 0 | 3 | 3 | 3 |
| Only work the head | 3 | 0 | 3 | 3 | 3 |
| Not so deep | 0 | 0 | 3 | 3 | 3 |
| Use the whole length, tip to base | 3 | 0 | 0 | 3 | 3 |

- The numeric modes' remaining misses are matters of degree. "Not deep
  enough" and "slower and deeper" moved the bottom turn 10–25 points deeper
  but stopped short of the base the check requires. With words, Gemma chose
  the base.
- Accents still stand in for every-stroke requests: Groove answered "all the
  way down" with frequent plunges, and Stroke ends answered "whole length"
  with frequent full strokes, so about a quarter of the strokes reach the
  base. The Groove replies said the groove stayed at the top; the Stroke ends
  replies described the added full strokes.
- Layered's nine rejections are the band problems the depth review
  describes: tip anchoring with strokes 100 points wide, and 85–95 and 45–55
  bands narrower than the shortest stroke. Its other misses widened or lowered the
  band, or chose full-and-base geometry inside the old 45–100 band, without
  reaching the base.
- Creative v2 starts from a score that already spans 5–95. In Compare modes,
  "Deepthroat it" sometimes changed nothing while the reply claimed more depth.

These are three trials per case with one model, covering depth language only.
The check measures where strokes turn, not how the motion feels. Each
contract's examples show tip work and deep work, so the suite partly measures
how well those examples transfer. The Lab sends no persona, speech register
or chat history.

## Visual review

The motion atlas rendered all 216 results of both runs from the shared
engine, 51 distinct outputs. Stroke scores reverse smoothly, and the planned
path and whole-percent wire output agree. Velocity and acceleration stay
continuous at every knot; the largest knot jumps are zero. Drift and roam move
a turn slowly over tens of seconds, accents appear as isolated plunges or full
strokes, and pauses and lingers show as short holds at a turn. The first Groove
names show "not going deep enough" as flicks between about 88 and 100.

Loops repeat every 64 strokes. Short tip work therefore repeats every 16–30
seconds, in Creative v2 as well as in the stroke score, while full strokes loop
over 120–220 seconds. Evolution edits and Autopilot give fresh realizations,
but a long hold on tip work will repeat audibly and physically.

## Open questions

- Which vocabulary feels best over a whole session, including Autopilot. The
  depth suite cannot answer this.
- Whether loop length should follow time rather than a stroke count, for both
  engines.
- Whether accents should stay available for every-stroke requests or be
  described only as occasional exceptions.
- What promoting a vocabulary to main chat would take: Autopilot planning and
  speech facts, mode switching, persistence and saved modes. None of that is in
  this change.

## Reproduce

From the repository root, with a loopback llama.cpp server:

```powershell
$env:MAGICHANDY_EVAL_URL = 'http://127.0.0.1:<port>'
$env:MAGICHANDY_EVAL_MODEL = '<model name the server reports>'
$env:MAGICHANDY_EVAL_REPEATS = '3'
$env:MAGICHANDY_EXPERIMENT_CAPTURE = '.scratch\lab-depth.json'
go test -tags liveeval ./internal/chat -run '^TestLabModesDepthLanguageLive$' -count=1 -v -timeout 60m
go run -tags magichandy_labs ./cmd/motion-atlas -output .scratch/lab-atlas.json -catalog=false -legacy=false -llm .scratch/lab-depth.json
python scripts/render-motion-atlas.py .scratch/lab-atlas.json .scratch/lab-atlas
```

`MAGICHANDY_LAB_METHODS` selects modes and `MAGICHANDY_EVAL_CASES` selects
cases, both comma-separated.
