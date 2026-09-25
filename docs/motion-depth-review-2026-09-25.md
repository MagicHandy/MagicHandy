# Motion depth review — 2026-09-25

## Finding

A Creative v2 session in the Explicit voice misread depth twice. After a
request to take the motion deeper, the accepted score kept every stroke
between about 41% and 94% of the slider, the shallow half, while the reply
described going all the way down. Autopilot's next stretch ran between 21%
and 79%. When the person said it was not deep enough, chat widened the
strokes to about 7–93%, and its reply described bottoming out on the tip.
Strokes bottom out at the base, so the reply contradicted the motion.

The prompts explain the misreading. The catalog and Creative contracts say
which end is deep: the tip is the shallow end and the base the deep end.
The Creative v2 and Layered contracts named the ends only by number: 0 is
the base and 100 the tip. Nothing told the model that deeper means lower
values. Requests in the slider's own words, such as "down" or "tip", mapped
correctly. Words from the scene, such as "deep", "bottom out" and "not so
deep", often went the other way, while the reply narrated the scene as
intended.

Layered has a second gap. Its anchors, and the base and tip in its geometry
names, act at the ends of the outer band, which the model chooses and often
narrows. Base anchoring inside a 45–100 band never goes below 45. The
contract did not say so.

## Changes

- **One depth frame.** A code-owned `DEPTH FRAME` section follows the
  response contract in every prompt that can move the device: chat, Autopilot
  planning and Autopilot check-ins, in all four motion vocabularies. Every
  position is a depth along one stroke: 0 is the base and deepest point, 100
  the tip and shallowest, and deeper means lower values. Depth in any scene
  the reply describes is depth along this stroke, and the reply describes
  only the depth the strokes actually reach. The frame defines depth; it
  matches no words in the request.
- **Motion facts.** Autopilot's motion domain line says which end is deepest.
- **Fields in depth terms.** Creative v2 describes range as the deepest and
  shallowest points any stroke can reach, focus position 0 as the band's
  deeper end, and mix by where strokes travel: full strokes cross the whole
  band, local strokes only their width. Layered says that the band, the
  anchor and stroke width decide depth together, and that base and tip in
  geometry names are the band's ends.
- **Geometry checks by effect.** A Layered geometry that removes center or
  range movement now accepts a layer on that axis at zero amount, which moves
  nothing. With the frame, Gemma often stilled the center layer beside base
  anchoring, and the old presence check rejected those replies.
- **Stroke width wording.** Layered's stroke width field names which bound
  is the shortest and which the widest stroke, and says that setting both to
  one value gives every stroke that width. See
  [request regression](#request-regression) for why.
- **Lab parity.** The LLM Lab's Creative v2 and Layered prompts include the
  frame.
- **Evaluation.** `TestContinuousDepthLanguageLive` compiles every accepted
  score through the shared engine and measures where strokes actually turn,
  instead of trusting the reply.

## Measurements

### Setup

- llama.cpp on an RTX 5070 Ti, one slot, 32,768-token context: the reporter's
  installed managed runtime, idle at the time, over loopback.
- Gemma 4 12B IT heretic Q4_K_M (SHA-256 prefix `239ec3629639`), the
  reporter's model, with the reporter's chat settings: reasoning off, 512
  output tokens, Explicit voice, custom anatomy with no wording, no persona
  and no memories. Saved speed 6–31, stroke 0–100, Original Handy.
- Each case starts from its own running score while Autopilot composes, so a
  rejection cannot affect later cases. Chat cases send one request. Autopilot
  cases plan the stretch 25 s after "Deepthroat it", with chat's short reply
  in history: one starts from a score that stays above 45 (recover), one from
  a score where every stroke reaches the base (keep).
- Each result is compiled by the shared engine and sampled at 50 Hz for
  90 s. Positions are commanded semantic values; no device was used.
- Five repeats per case and build. The baseline is `main` at `cf0eff76`
  (alpha.49) with the same test file.

A stroke's turn is where it reverses by at least 2 points. The criteria are:
**deep**, the deepest point at most 12 and at least half the strokes turning
within 20 of the base; **deeper**, the deepest point at most 12 and the median
turn at least 10 points lower than before; **shallow**, no stroke below 50;
**less deep**, the median turn at least 10 points higher; **whole length**,
reaching below 10 and above 90 with at least half the strokes spanning 70
points. A rejected reply or an unchanged score fails.

### Results

Passes per request, five trials each. Every case starts from a 45–100 band
except "bottom out" (25–90), the tip requests (5–95), "not so deep" (0–55)
and the Autopilot keep case, whose strokes already reach the base.

| Request | Criterion | Creative v2 `main` | Creative v2 final | Layered `main` | Layered final |
| --- | --- | ---: | ---: | ---: | ---: |
| "Deepthroat it" | deep | 0/5 | 4/5 | 0/5 | 0/5 |
| "you're not going deep enough" | deeper | 1/5 | 5/5 | 0/5 | 5/5 |
| "Take it all the way down every time" | deep | 5/5 | 5/5 | 0/5 | 0/5 |
| "Bottom out on every stroke" | deep | 2/5 | 5/5 | 0/5 | 0/5 |
| "Slower and deeper" | deeper and slower | 0/5 | 5/5 | 0/5 | 4/5 |
| "Just the tip for a while" | shallow | 5/5 | 5/5 | 0/5 | 0/5 |
| "Only work the head" | shallow | 5/5 | 5/5 | 0/5 | 0/5 |
| "Not so deep" | less deep | 0/5 | 5/5 | 0/5 | 0/5 |
| "Use the whole length, tip to base" | whole length | 1/5 | 5/5 | 0/5 | 0/5 |
| Autopilot, score still above 45 | deeper | 0/5 | 0/5 | 0/5 | 0/5 |
| Autopilot, strokes at the base | deep | 5/5 | 5/5 | 5/5 | 5/5 |

A right-way move shifts the median stroke turn at least 5 points in the
requested direction:

| 45 chat turns per mode | Creative v2 `main` | Creative v2 final | Layered `main` | Layered final |
| --- | ---: | ---: | ---: | ---: |
| Met the criterion | 19 | 44 | 0 | 9 |
| Moved the right way | 34 | 45 | 25 | 35 |
| Moved the wrong way | 5 | 0 | 5 | 1 |
| Rejected | 0 | 0 | 8 | 9 |

On `main`, all five Creative v2 answers to "Deepthroat it" moved the
strokes up to about 63–84%, at or near the top saved speed: the reported
inversion. "Not so deep" confined every stroke to the deepest 30 points in
all five trials. After the change, every Creative v2 turn moves the right
way; the one miss deepened to 20 instead of reaching the base. Layered
moves the right way more often. On `main` it inverted every "bottom out"
with tip anchoring; after the change it inverted once, choosing tip
anchoring for "all the way down". It meets the strict criteria less often,
for the reasons in the follow-up below.

Autopilot kept depth that chat had established in every trial on both
builds. When chat had answered but the score still stopped at 45, no
planning turn on either build lowered the band: they changed pace or
variation, refreshed the seed, or, in Layered after the change, anchored
at the band's floor.

### Replies

On `main`, 12 replies spoke of bottoming out, and 9 of them came with no
stroke within 15 points of the base, like the reported reply. After the
change, 14 did, and 4 had no stroke that deep. All four are Layered
"bottom out" turns anchored at the band's floor of 25.

### Request regression

`TestContinuousRequestLive` (37 cases across both modes, two repeats)
passed 74 of 74 on both builds. Its holdouts passed 55 of 58 on `main` and
56 of 58 after; every miss is in the Creative v2 mixed-base and pace-where
cases.

An intermediate build failed all six Layered stroke-width trials: asked to
set the shortest and widest stroke to exactly 30, Gemma set a 10–30 range.
The same happened with an unrelated paragraph in the frame's place, so
`main` had passed those cases by a narrow margin; the frame did not change
their meaning. Naming which bound is which, and saying that setting both to
one value gives every stroke that width, passed 18 of 18 width trials.

### Visual review

The motion atlas rendered every result of both builds from the shared
engine. On `main`, the Creative v2 answers to "Deepthroat it" were short,
fast strokes between about 63% and 84%, the pattern in the reported trace,
and two answers to "bottom out" never went below 20–25%. After the change, the same requests compile to full strokes
that reach the base, or to base-anchored work mixed with full strokes. "Just
the tip" and "only the head" stay between about 70% and 95%, and "not so
deep" lifts the strokes off the base. Layered depth requests that stop short
show as flat bottoms at the band's floor, 45% or 25%, and Layered tip
anchoring keeps strokes that still reach about 25%. Reversals stay smooth,
and no new artifacts appear.

## Layered band follow-up

After the change, Gemma's Layered replies usually point the right way:
base anchoring, full-and-base alternation or a lower band for depth, and
tip anchoring for the tip. The remaining failures read anchors and stroke
widths as absolute. The model anchors at the base inside a band that starts
at 45 or 25, asks for strokes wider than the band, raises the deepest point
past the shallowest, or anchors at the tip while keeping strokes wide
enough to reach about 25. The parser rejects or confines those edits as
the contract says.

Closing the gap changes Layered semantics, which is a decision rather than a
prompt fix:

1. **Name the physical ends.** Base- and tip-named geometries would extend
   the band to 0 or 100 unless the same edit sets that end. This matches the
   frame, but an Autopilot geometry choice would then move the band, which
   could undo a region the person asked for.
2. **Absolute widths and anchors.** Express stroke width and anchor in slider
   positions instead of band-relative values, so the model's reading is the
   engine's. This changes the score format and every Layered example.
3. **Keep the band-relative contract** and accept the gap for small models,
   as ADR 0023 prefers visible shortcomings to implicit corrections.

This change keeps option 3.

## Not selected

- **Layered depth examples.** Two examples paired a band change with an
  anchor. In three-repeat runs they raised strict Layered passes from 5 to 10
  of 27, but rejections rose from 3 to between 10 and 15, and right-way moves
  fell from 24 to between 12 and 17. Gemma copied the pattern into bands
  where its values were invalid, and a rejected continuous turn is not
  repaired.
- **Any word rule**, such as mapping particular phrases to the base, per the
  user's direction. The frame defines depth once for every phrasing.
- **Autopilot correcting chat.** With chat's reply in history, no planning
  turn lowered the band under a score that chat had left short, on either
  build. Now that chat reads depth correctly, a planning turn rarely
  inherits such a score, so this was not pursued.

## Limits

- One model in one configuration, five repeats per case. The requests are
  English; other reply languages share the English contract and frame but
  were not measured.
- Creative v2 can deepen only partway: one of 45 turns stopped at 20
  instead of reaching the base.
- Autopilot keeps depth that chat established, but did not correct a score
  that chat left short.
- Layered reaches the base less often than it moves toward it; see
  [the follow-up](#layered-band-follow-up).
- Replies still sometimes describe more depth than the strokes reach.
- The frame uses the semantic convention that 0 is the base. Reverse
  direction and the physical stroke window remain backend settings and are
  unchanged. Positions are commanded estimates; physical acceptance remains
  open.

## Reproduction

From the repository root, with a loopback llama.cpp server:

```powershell
$env:MAGICHANDY_EVAL_URL = 'http://127.0.0.1:18180'
$env:MAGICHANDY_EVAL_MODEL = '<model name the server reports>'
$env:MAGICHANDY_EVAL_REPEATS = '5'
$env:MAGICHANDY_EXPERIMENT_CAPTURE = '.scratch\depth.json'
go test -tags liveeval ./internal/chat -run '^TestContinuousDepthLanguageLive$' -count=1 -v -timeout 60m
```

`MAGICHANDY_EVAL_CASES` selects cases by name, and `MAGICHANDY_EVAL_VOICE`
changes the voice. The report keeps each raw response, reply, score and
depth measurement, in the motion atlas's LLM report format:

```powershell
go run -tags magichandy_labs ./cmd/motion-atlas -output .scratch/depth-atlas.json -catalog=false -legacy=false -llm .scratch/depth.json
python scripts/render-motion-atlas.py .scratch/depth-atlas.json .scratch/depth-atlas
```
