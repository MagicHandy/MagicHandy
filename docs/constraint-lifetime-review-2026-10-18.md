# Hosted constraint lifetime and context review — October 18, 2026

## Finding

The stronger active-direction guidance was sometimes too conservative about a
one-time pace setting. It did not generally freeze motion character. Both
Layered and Creative v2 continued changing reach and texture at a fixed base
pace, and explicit timed directions and releases usually worked already.

The final matched comparison uses ChatGPT `gpt-6-sol`, reasoning off, a 131,072
token application admission window, saved speed limits 10–50 and seed 841.
Each pair shares the same real interactive reply and starting score. Four
successive planning turns occur at simulated request ages 15, 90, 180 and 360
seconds; accepted scores, recent speeds and compiled motion facts carry forward.
Only the hosted lifetime paragraph differs between the two conditions.

| Human direction | Mode | Previous guidance: base pace at the four decisions | Clarified lifetime |
| --- | --- | --- | --- |
| Set the speed to 25 percent now | Layered | 25, 25, 25, 30 | 30, 28, 25, 20 |
| Set the speed to 25 percent now | Creative v2 | 25, 25, 25, 25 | 25, 29, 27, 25 |
| Use 15 percent for the next minute, then continue developing | Layered | 15, 22, 28, 34 | 15, 23, 29, 34 |
| Same one-minute direction | Creative v2 | 15, 23, 29, 34 | 15, 24, 28, 33 |
| Keep the base speed at 25 throughout the session; develop the rest | Both | 25, 25, 25, 25 | 25, 25, 25, 25 |

The initial chat applied the one-time 25% request before these decisions.
Neither condition froze the other dimensions: all four subsequent scores in
each of the six pairs changed character beyond seed and base speed. In the
revised lasting-pace case, mean stroke length varied 46.4–59.9% in Layered and
33.6–62.7% in Creative v2 while base pace stayed at 25. More numeric edits or a
higher variance statistic are not, by themselves, better motion.

This is a small stochastic sample of synthetic sessions on one account/model,
not a naturalness rating or proof that every ambiguous instruction is resolved
correctly. A one-time setting has some inherent ambiguity while Autopilot owns
the session. The useful improvement is distinguishing its lifetime from an
explicit session-long preference, without requiring a particular next speed.

## Changes

- Hosted continuous planning now distinguishes one-time settings, temporary
  intervals, feedback and lasting restrictions. It first satisfies a requested
  stretch, resumes existing autonomy when that interval ends, and retains
  genuinely lasting directions. Open dimensions may develop; no dimension
  must change every turn. Interpretation stays in the model. There is no
  keyword rule, inferred lock, fixed expiration timer or additional model call.
- Named centered/base/tip geometry fits an inherited stroke-width envelope
  into an explicitly narrowed outer band. It preserves a smaller envelope,
  zero-as-full-band semantics and independent pace controls. Previously the
  named-operation grammar could not repair that inherited width, causing valid
  band/anchor choices to fail validation and leave the previous score running.
- The hosted contract clarifies that a drifting range layer does not replace
  the base `variation_mode`. One retained regression chose a drift layer while
  leaving a requested replacement of periodic base variation undone.
- Setup and Settings expose **External context budget** per connection.
  **History sent**, under the usage disclosure, can be shorter than that budget.
  A 128K request budget can therefore keep Recent history (64 messages / 24 KB)
  while leaving room for instructions, structured output and planning state.
  History is capped by the selected budget. An omitted `history_size` preserves
  the existing behavior; saved explicit budgets are not silently upgraded.

The existing catalog recommendation remains 128K for known hosted models with
sufficient advertised capacity. Unknown/manual and compatible models are not
assumed to support it. A larger admission budget cannot increase a model's
actual capacity. Local context settings, local prompts and history limits are
unchanged; no local inference call or runtime dependency is added. The semantic
geometry fit is shared by both contracts; the engine, transports and Stop path
are unchanged.

## Evidence and limitations

The final matched run has **54/54 valid checked responses**: six shared real
chat acknowledgments and 48 autonomous decisions. Separate checks passed
**7/7** standing-hold decisions and **18/18** final Layered regressions,
including paraphrased/Spanish directions, a ten-minute lasting pace, release,
fixed band, exact hold and named/custom geometry. These 79 responses used one
provider call each without repair or fallback. Observed latency was
2,526–11,160 ms, median 3,680 ms; this is not a controlled provider benchmark.

Earlier runs are retained, not folded into a claimed final success rate:

- Initial 32K screening: 72 attempted decisions; 20 were rejected by local
  input admission, before model generation. Full Layered state can exhaust the
  remaining prompt allowance after schema/output reserves. Such a failure
  preserves the previous motion and can look like excessive persistence.
- Larger-context baseline: 69/72 parsed Layered decisions and 36/36 Creative v2
  decisions. Three Layered failures exposed the inherited-width problem.
- Candidate screening: 72/72 parsed; a separate acknowledgment follow-up
  completed 40/40 chat/planning turns.
- An initial regression sweep was 17/18 on intent: the periodic base-variation
  case above remained wrong despite its valid schema. The clarified technical
  description was followed by the final 18/18 sweep.

The preliminary harness had two assessment defects. Its first easing fixture
incorrectly reported two minutes already spent at a newly requested speed.
It also initially rounded stroke CV as a fraction rather than the percentage
used by production. The final matched run corrects both and shares an actual
chat acknowledgment between variants. Earlier reports and their original
flags remain unchanged. Seven preliminary flags additionally imposed an
unjustified minimum duration on the vague word "brief"; that case is now
observational. Explicit one-minute requests retain their timed assertion.

## Compiled motion review

The combined atlas retains **389 records**, including admission failures and
wrong model selections. The shared compiler produced **311 distinct plots**
and **20 overview sheets**; all overviews were inspected. Detailed comparisons
cover the one-time pace cases before/after, fixed-pace Creative v2 development,
and the failed/final irregular-reach selections. Fixed pace still has clear
width/location development. Planned and whole-percent trajectories track
closely in the inspected excerpts, without new flattened turns or obvious
quantization chatter. A smooth curve does not excuse the wrong selected mode.

All evaluations compile semantic targets without an engine or transport. No
real device was connected or moved. The plots describe steady commanded
motion, not physical feedback, a six-minute dispatch trace, or a new test of
retargeting/Stop. Existing lifecycle and race tests remain the safety checks.

Ignored artifacts are under `.scratch/constraint-lifetime/`: all named JSON
reports, `combined.json`, `atlas.json`, `atlas/index.html`, the comparison PNG,
test logs and `final-review/` readiness/chat evidence. The prior hosted paragraph
is a committed synthetic fixture for repeatable comparisons, not runtime text.

To reproduce the matched run, set `MAGICHANDY_HOSTED_DATA_DIR` to an existing
signed-in profile, `MAGICHANDY_LIVE_MODEL=gpt-6-sol`,
`MAGICHANDY_LIVE_CONTEXT=131072`, `MAGICHANDY_CONSTRAINT_VARIANTS=current,previous`,
`MAGICHANDY_CONSTRAINT_CASES=one_time_speed,timed_easing,lasting_speed`,
`MAGICHANDY_CONSTRAINT_CHAT_SETUP=1`, and `MAGICHANDY_EXPERIMENT_CAPTURE` to an
ignored JSON path. Run `go test -tags liveeval ./internal/chat -run
'^TestConstraintLifetimeLive$' -count=1 -v`. Both modes run by default; use
`MAGICHANDY_CONSTRAINT_MODES` to narrow them. These tests consume plan usage.
Run `TestConstraintLifetimeChatLive` and `TestHostedLayeredLivePlanning`
separately with separate report paths. Render captures with the documented
`motion-atlas -catalog=false -llm ...` workflow.

## Verification and review app

Full Go tests and race suite, vet, standard golangci-lint, CGO-free build,
frontend typecheck/build, localization audit and all **915 frontend tests** pass.
The final hosted technical clarification also passed the affected Go packages
and a repeated chat race check. An additional `liveeval`-tag lint audit reports
the same 20 pre-existing findings as the parent commit, with none in the new
lifetime harness; no gate or exclusion was changed.

The current isolated simulator is at `http://127.0.0.1:50260/#/settings/chat/model`,
showing saved 128K / Recent controls for an unassigned ChatGPT connection. No
credentials were copied. The active local Gemma model passed the real readiness
probe and a text-only app chat in 1,082 ms, one call, with no repair, fallback or
motion. Real ChatGPT evaluation used the host's existing sign-in separately.
See the scorecard for measured binary/UI size changes.
