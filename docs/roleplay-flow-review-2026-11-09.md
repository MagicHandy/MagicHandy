# Roleplay flow review — 2026-11-09

## Question

Do the persona presets keep dialogue and motion matched and flowing in
roleplay when the person makes no specific motion request, and do all presets
and roleplay types work with the external models in use? The models are
GPT-6.1 Terra through a ChatGPT sign-in (reasoning low) and DeepSeek V4.1 Flash
1017 through OpenRouter (Advanced, no reasoning, fastest-output routing), both
on alpha.83 in Layered.

## Method

`TestHostedRoleplayFlowLive` (`liveeval`) plays scenes in which the person
reacts, invites and gives cues but never names a speed, depth or pattern. It
uses the production Advanced chat path: the persona's voice, reaction style and
description, balanced replies, one repair attempt and saved speed limits 1–100.
Each turn records the reply, the motion action and the compiled feel before and
after: mean commanded travel, the lowest point reached and the mean stroke
length.

- **Preset matrix.** All 24 combinations of chat voice (utility, warm,
  intimate, explicit) and reaction style (neutral, playful, tender, dominant,
  submissive, teasing) play one seven-turn arc. It starts stopped: a greeting,
  an invitation ("I'm all yours, whenever you're ready"), pleasure, "Please, I
  want more", "I'm getting close", "Don't you dare stop now", and a wind-down
  ("That was incredible. Stay with me for a minute.").
- **Roleplay types.** Six five-turn scenes pair a kind of roleplay with a
  persona that suits it: tease and deny (explicit, teasing), being in charge
  (intimate, dominant, starting stopped), following the person's lead (warm,
  submissive), slow romance (intimate, tender), playful banter (warm, playful)
  and a rough scene (explicit, dominant). Only the rough scene uses roleplay
  words for intensity ("Harder…").

`TestRoleplayFlowJudge` uses GPT-6.1 Terra for every run. It labels each reply
from its text alone, without the motion data: does the stroking start or stop
now, go faster, slower or steady, deeper or shallower, longer or shorter?
Those labels are compared with the compiled change, using thresholds of ±10%
mean travel, ±5 points for the lowest position and ±15% stroke length. A
described change counts as made, missed or contradicted. "All the way down"
while the motion already reaches the base counts as consistent. The judge then
rates each scene from 1 to 5, seeing what the device actually did:

- **coherence:** reply and motion tell the same story;
- **responsiveness:** the motion answers the person's cues;
- **character:** the motion choices fit the persona's style.

## Findings before the change

- **GPT-6.1 Terra mostly stayed static.** The motion was unchanged in 86% of
  turns. Terra started on the invitation in 2 of 24 scenes; it usually started
  only at "I want more". It never responded to "I'm getting close", and never
  eased off at the wind-down: the device kept running while replies said "I'm
  right here beside you" and "catch your breath". In the rough scene the
  device stayed at 75% after "I need a second" while the reply said "Take your
  time". Replies described touch beginning ("I'm putting my hands on you") with
  the device still stopped.
- **DeepSeek V4.1 Flash moved more but did not finish.** It started on the
  invitation in 18 of 24 scenes and built through the arc, but eased off at the
  wind-down in only 10 of 24. It kept running at up to 100% while replies said
  "catch your breath". It also promised starts it did not make ("I'm going to
  start slow"). Of the changes its replies described, 17% were missed or
  contradicted.
- **The prompt pulled the two apart.** The warm, intimate and explicit voice
  checks ask for replies about embodied action. The motion contract told the
  model that ordinary conversation requires reply only, that conversation
  requires empty edits, and that a stopped device starts only for a direct
  motion request. Roleplay reactions read as conversation. Reaction styles
  deliberately never mention motion, so nothing connected a persona's character
  to what the device does.
- **Presets work on both models.** No preset or roleplay type caused a
  refusal. Warm and intimate replies never used explicit terms. Terra's
  explicit voice used explicit terms in only 21% of turns, against 60% for
  DeepSeek.

## Change

See [ADR 0067](decisions/0067-advanced-chat-keeps-scene-reply-and-motion-in-step.md).
Advanced live chat gets one persona-agnostic scene guide: reactions are motion
feedback, the reply and the motion must agree, finishing or catching breath
eases off, and holding someone off is slower or lighter. A stopped device
starts on an invitation to begin but never for a reaction to earlier motion.
Empty edits are for questions and talk that is not about the motion. Simple
and Intermediate prompts, reaction styles, voices, limits and Stop are
unchanged.

Four versions were tried, each on Terra first:

1. **The guide alone.** Terra started on every invitation and built on "I want
   more", but still kept running at the wind-down in 19 of 24 scenes, while its
   replies described holding the person.
2. **Finishing and catching breath as cues.** Adding them, and forbidding a
   resting reply while the motion keeps its pace, fixed the wind-down: 23 of 24
   scenes eased off.
3. **Holding off as slower or lighter motion.** Teasing personas still often
   chose to let the person finish, which is consistent and in character.
   Coherence rose slightly.
4. **Without "within the saved limits".** The earlier wording told the model to
   answer "within the saved limits". The regression suites showed that it then
   kept "full-range" sweeps inside the current 5–95% band in every repeat (0 of
   8, against 5 of 12 on alpha.83). Saved limits are already stated and
   enforced elsewhere, so the phrase was removed. This is the final version.

## Results

Terra's "after" column is the mean of two passes of the final version (scene
ratings 3.83 and 4.23 for coherence), which shows how much a single pass
varies. DeepSeek's is one pass.

| | Terra before | Terra after | DeepSeek before | DeepSeek after |
| --- | --- | --- | --- | --- |
| Coherence (1–5) | 2.83 | 4.03 | 3.03 | 3.97 |
| Responsiveness (1–5) | 2.40 | 4.47 | 3.73 | 4.87 |
| Character fit (1–5) | 2.93 | 4.22 | 3.63 | 4.10 |
| Turns with unchanged motion | 86% | 48% | 41% | 24% |
| Described changes made | 19 of 21 | 210 of 238 | 111 of 134 | 162 of 206 |
| Described changes contradicted | 0 | 7 | 9 | 13 |
| Starts on the invitation | 2 of 24 | 48 of 48 | 18 of 24 | 24 of 24 |
| Builds on "I want more" | 19 of 24 | 47 of 48 | 24 of 24 | 24 of 24 |
| Eases off at the wind-down | 0 of 24 | 47 of 48 | 10 of 24 | 24 of 24 |
| Explicit terms, explicit voice | 21% | 67% | 60% | 64% |
| Errors, repairs, refusals | 0, 0, 0 | 0, 2, 0 | 2, 1, 0 | 0, 0, 0 |

- **Every voice and style improved on both models.** Terra now describes and
  makes changes. DeepSeek still makes about four in five of the changes it
  describes, as before. Its gains are in answering the scene at all and in the
  moments that matter most: starting on an invitation and easing off at the
  end.
- **Weakest character fits.** After the change these are teasing (Terra about
  3.4, DeepSeek 3.0) and playful (about 3.5 and 3.25). Teasing personas often
  let the person finish instead of holding them off. That is consistent with
  the reply, but less in character.
- **Explicitness.** Warm and intimate replies stayed free of explicit terms on
  both models. The explicit voice is more explicit in practice, because
  replies now describe the stroking itself.

### Cold, specific requests

Terra ran all 24 hosted live suites (17 default, six opt-in and the
follow-through sessions) with the production variants, through the ChatGPT
sign-in.

- **Terra, all suites.** Alpha.83 passed 327 of 372 intent checks and the final
  version 331. Seven suites gained a case and six lost one, within run-to-run
  variation; repeated runs of the four suites that dropped came back level.
- **Terra, compound motion.** It is 42 of 72 over six repeats, against 45 of 72
  on alpha.83. The remaining gap is the regional-timing case, which is already
  weak (2 of 12, then 0 of 12).
- **DeepSeek, nine focused suites.** These cover stopped-motion intent,
  conversation versus edits, praise, natural conversation, output awareness,
  conversation depth, unapplied history and Creative pace. They went from 94 to
  93 of 110. The difference is rate-limit errors: every case that reached the
  model passed in both. An earlier DeepSeek run hit an OpenRouter outage (69 of
  110 calls failed at the provider) and was repeated.

### Other modes

| | Creative v2 before | Creative v2 after | Autopilot composing before | Autopilot composing after |
| --- | --- | --- | --- | --- |
| Coherence | 3.07 | 4.20 | 3.13 | 4.17 |
| Responsiveness | 2.57 | 4.17 | 2.77 | 4.50 |
| Character fit | 3.30 | 4.40 | 3.23 | 4.27 |
| Starts on the invitation | 1 of 24 | 22 of 24 | 15 of 24 | 24 of 24 |
| Eases off at the wind-down | 0 of 24 | 22 of 24 | 0 of 24 | 23 of 24 |

Both are Terra. The Creative v2 run used version 3; the Autopilot run used the
final version. "Autopilot composing" is chat while Autopilot runs; the
planner's own decisions between turns were not simulated. Creative v2 had 8
and 9 rate-limit errors in its two runs, from running many Terra processes at
once.

### Visual review

Terra's roleplay outputs were exported through the shared engine with
`cmd/motion-atlas` (198 cases per run) and rendered with
`scripts/render-motion-atlas.py`. Before the change there were 25 distinct
motions; after it there were 75. Every overview sheet of the candidate was
inspected:

- Builds widen and quicken.
- Wind-downs narrow to short, centred strokes at 1–20%.
- There are no stalls, collapsed bands or knot jumps.

Two choices are extreme but within the saved limits: a teasing slowdown to the
1% saved minimum, which is practically still, and full-length strokes kept at
15% after "I need a second".

One existing weak spot showed up. A reply claimed more pace while the model's
new pace layer, with a 1% floor, cut mean travel from 113 to 8 %/s. The
contract already warns that pace layers and short strokes can slow motion more
than base speed suggests.

## Not covered

- **Physical feel.** All evidence is replies and compiled motion.
- **Autopilot's own planning between chat turns** during a scene.
- **Simple and Intermediate local models.** Their prompts are unchanged and were
  not re-run.
- **Non-English prompt sets.** The guide is English, like the rest of the
  contract.
- **The judge.** It is one model. Scene ratings for the same prompt varied by
  up to 0.4 between runs.


## Independent release review and corrections

The alpha.84 review used a byte-verified copy of Claude's uncommitted
`claude/roleplay-flow` changes on top of alpha.83 (`4cc745d0`). The scene
guidance is retained. Its reported improvements are directional evidence from
one judge, not a guarantee that every description matches the resulting pace.
In particular, the ratio of implemented descriptions did not improve uniformly
in the larger runs even though responsiveness and scene ratings improved.

The review found a contract mismatch: the new guide invited complete rests,
but continuous live chat could emit only `none`, `start` or `update`. Lowering
speed is not Stop. Advanced live Layered and Creative v2 now offer `stop` with
empty edits. It emits the existing bare Stop command through the shared engine
and ends Autopilot. It cannot carry simultaneous vibration edits or a motion
score. It is also accepted while paused; it cannot resume anything. The
scheduled Autopilot contract and Simple/Intermediate contracts are unchanged.
This is model-selected action interpretation, with no new language matcher.

Evaluation defects were fixed before release:

- Failed generations, empty replies, failed compilation and failed judge calls
  fail their tests and retain the failure evidence. Rejected assistant text is
  excluded from subsequent conversation history.
- A stopped turn no longer exports the remembered moving score for plotting.
  Contract acceptance is no longer labelled an intent pass.
- Judge objects are checked for missing/extra fields, invalid enum/type values
  and out-of-range ratings. Failed source turns are not scored as delivered
  replies.
- The existing lifecycle checker now accepts both supported counted-phrase
  formats. Its first compound case produced a correct two-short/one-full
  `regions` response, which the old `strokes`-only checker incorrectly failed.

Fresh, non-graphic Utility checks through ChatGPT GPT-6.1 Terra (low reasoning,
131072 context) passed **27/27** lifecycle cases in Layered and Creative v2.
They cover fresh requests, invitations, post-Stop reactions, prospective
requests, questions, quoted requests, pause, disabled motion and a script owner.
Two additional five-turn conversations passed without repair or fallback:
greeting stays stopped, invitation starts, keep-unchanged preserves motion,
finish emits Stop, and the following thanks stays stopped. App-path fake
transport tests confirm model-selected Stop and a later fresh restart use the
same engine. Focused race tests include Stop and late-response boundaries.

The first attempted live run had the wrong context/profile configuration and
never established a usable model test; it is retained separately, not counted
as model failures. The final runs use the available review profile in place,
without copying its credentials.

The retained Claude captures were checked directly: final Terra runs each have
198 accepted turns and one repaired turn; final DeepSeek has 198 accepted turns
with no repairs. The DeepSeek baseline has two failed requests. Median captured
latency rose from **2967 ms to 3694/3654 ms** for Terra and **1076 ms to 1883 ms**
for DeepSeek. These are end-to-end trial times, not controlled provider latency
benchmarks. No inference round is added to ordinary successful requests.

All 594 final retained proposals were recompiled with the shared engine. The
corrected atlas contains 262 distinct plots and 17 overview sheets, all reviewed,
plus detailed outliers 4, 28, 198 and 493. Reach variation and rounded turns
remain distinct. A 1% slowdown is genuinely near-stationary; another 100% base
speed proposal averages only 7.5 percent of travel per second because of its
selected timing. Those remain model-selection limitations, not evidence of a
new sampler defect. No transport, fitter, sampler, device limits or previously
physically selected timing changes are included in this update. No hardware
was connected during this review.

Artifacts remain ignored under `.scratch/alpha84-review/`: retained raw
captures, `claude-final-compiled-input.json`, `claude-final-atlas.json`,
`claude-final-atlas/index.html`, lifecycle and feedback captures, unit/race/lint
logs and review-app probe results. The five-turn release checks are additional
evidence for the Stop correction; the larger judge scores above predate it and
are not presented as a new benchmark of that correction.


Final validation: `go test ./...`, `go vet ./...`, `golangci-lint run ./...`,
CGO-disabled core build, the opt-in harness unit checks, and focused race checks
for the new Stop path and existing Stop/late-response boundaries pass. The 37
fresh live turns also compile into a separate nine-plot atlas. The isolated
current-source app at `http://127.0.0.1:50358/#/chat` passed
`scripts/check-review-llm.ps1` with available local Gemma 12B and a text-only
app chat request (309 ms, non-empty reply, no repair/fallback or motion).
Its simulator remains idle. These local checks accompany, and do not replace,
the complete CI and release-package gates.
