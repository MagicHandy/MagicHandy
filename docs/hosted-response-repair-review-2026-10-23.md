# Hosted response repair and Layered join review — 2026-10-23

## Installed-session finding

Reviewed the installed alpha.69 session and its diagnostic export without
taking controller ownership, changing settings, connecting a device, or issuing
motion. One failed request remained in that session's bounded failure log. It
used ChatGPT `gpt-6-sol`, reasoning off, and a 512-token response allowance.
Generation took 5.819 seconds and returned a complete 148-token JSON proposal.

The proposal put four upper strokes at 78–100 after two lower strokes at 0–35
in a repeating phrase. Each group was individually valid, but the loop joined
an upper turn at 35 to a purported lower turn at 78. That is an ascent, not the
descent required between alternating lower/upper turns. Rejection was correct;
it was neither a provider refusal nor a network/output timeout. Earlier
successful targets increased the selected speed through 25, 38, 52 and 100,
and retained Cloud trace commands succeeded. A later heartbeat expiry occurred
after playback was already stopped and does not explain the JSON rejection.
This evidence identifies the retained failure, not every error the user may
have encountered.

## Changes and alternatives

Keep the existing phrase grammar and shared motion engine. Validation now
identifies the failing group join, its actual endpoints, and whether it is the
loop seam. It does not modify the score or relax the minimum stroke span.
Hosted composition guidance explains the cyclic relationship explicitly:
uncounted regional alternation can use the existing field geometry; exact
counts can use an explicit connecting climb while retaining the requested
regional counts. A full sweep already requested by the user can supply that
climb. The model chooses the representation and coverage. No keyword-based
language correction, fixed regional width, fixed speed, or implicit extra
stroke is introduced.

The initial visual review also exposed a valid but poor field selection: very
rapid center alternation consumed the short regional strokes and produced
larger traversals. Guidance now explains allowing strokes within a region
before moving its center. Live evaluation checks actual compiled lower and
upper turns, not just the presence of a center layer. This is an evaluation
criterion, not a runtime rule that rewrites language or model choices.

[ADR 0053](decisions/0053-hosted-chat-response-repair.md) adds one optional
correction call for rejected structured hosted chat responses. Automatic repair
defaults on. When disabled, an eligible live chat error offers **Repair
response**. Both paths retain the original conversation and contract, submit
the rejected object and exact validator feedback to the same provider/model,
and validate the replacement normally. Successful replies still use one call.
No partial reply, speech, mood, or motion from rejected output is applied.

The continuation is single-use, lasts at most five minutes, and is invalidated
by a newer turn, session/control/model/settings change, Stop, or changed motion.
The repair cannot overwrite a newer engine target. Reloading the page does not
restore the temporary button. Failed/canceled repair evidence remains in the
bounded administrator diagnostics; it does not authorize applying the result.
Plain prose, typed refusals, empty output, network/authentication errors and
provider truncation are outside repair eligibility. A refusal during repair
cannot start a third call through local fallback.

## Claude review

Consulted the installed Claude CLI with `claude-opus-5-5`, effort `max`, for
motion design and a separate backend/UI review. Shared source and synthetic
mechanical examples, not the user's private conversation or credentials.
Claude agreed that the seam rejection is correct and the current grammar can
represent the request without redesigning Layered. It recommended model-chosen
connecting travel, an uncounted field alternative, and neutral engine errors.

The implementation also incorporates its findings about admission-before-
consumption, preserving the actual user sequence, shared metadata validation,
optional repair bookkeeping not breaking otherwise valid chat, preventing a
third fallback call, and compact application-owned failure/progress UI. The
error is not attributed to the persona; a successful repair becomes an ordinary
assistant reply. Diagnostics retain repair provenance without a persistent
warning bubble or duplicate error toast.

Two suggestions were deliberately deferred: restoring manual repair across a
page reload would need a new authoritative state projection, and relaxing
automatic repair's checks around sliders could apply a proposal based on
superseded controls. The first version keeps a live-only action and honors newer
controls for both repair paths.

## Fresh motion requests after Stop

The follow-up report identified a different cause of replies that merely said
the device was stopped. Continuous interactive instructions required a request
to begin movement and separately said that changing stopped settings never
authorized movement. A terse request such as "Use syncopation" could therefore
be interpreted as configuration without playback.

Interactive guidance now distinguishes a fresh request to perform motion from
a question or an explicitly prospective/settings-only request. With LLM motion
enabled and the device stopped, the model can emit `start` together with its
chosen motion, without requiring a separate start command. A new motion
request can supersede an earlier Stop; it does not revive the request or repair
that Stop canceled. Paused motion still requires app resume. Disabled LLM
motion, Script-owned playback, controller authority and saved limits retain
their existing restrictions. Autopilot gains no new start permission.

Creative and Pattern library also contained older word-based start filters.
For hosted interactive starts, the complete model-declared command now goes
through state/ownership/limit validation instead of depending on those word
matches. The server does not infer a start from user text or manufacture a
missing command. Local legacy authorization and unrelated running-update
behavior are retained. Continuous modes share the revised, compact interactive
wording; this does not add another inference call.

The live lifecycle matrix passes **25/25**, with one real `gpt-6-sol` call per
case, reasoning off, 32K context and 512 output tokens. Each of Layered,
Creative v2, Creative and Pattern library covers a fresh motion request,
motion after an earlier Stop, a question, future-only settings and paused
motion. Additional cases cover a counted request, quoted motion instructions,
ordinary conversation after Stop, LLM motion Off and Script ownership. Median
generation is **3.235 seconds**, range **2.622–5.985 seconds**. The final report is
`.scratch/hosted-response-repair/stopped-motion-intent-combined.json`; each row
names its source capture.

Testing against the active production Pattern catalog found two additional
rejections: the model selected no motion but supplied pattern/target fields.
The old schema advertised these fields for all actions. Hosted Pattern chat
now uses state-specific branches: none/stop carry no target fields, a stopped
start requires speed, and paused chat has no start branch. The parser remains
strict. The final active-catalog five-case run passes 5/5. Local grammar and
the autonomous Pattern schema remain unchanged; regression tests compare
hosted/local Autopilot schemas directly. The first full Go run caught an
Autopilot consumer's assumption about the flat schema, prompting an explicit
shared-base builder before the final suite.

The first lifecycle fixture used legacy test pattern IDs, and a subsequent
catalog enumeration included deprecated entries. Both reports remain alongside
the final capture; the final fixture uses the same active non-experimental
built-ins as production and consistent 1–100 limits. Compilation checks verify
that every final selected pattern ID resolves to itself, rather than a fallback.

App/fake-transport tests exercise start/Stop/fresh restart for all four modes,
deny attempted starts while paused/off/script-owned, and confirm that a fresh
start does not revive an earlier repair or start Autopilot. The shared engine
owns every accepted target.

All nine final moving proposals also pass production chat-target mapping and
shared-sampler compilation, without starting an engine or transport. The final
lifecycle atlas contains 25 records / seven distinct plots at
`.scratch/hosted-response-repair/lifecycle-atlas/index.html`. Older fixtures and
catalog rejections are retained in a separate combined lifecycle atlas
(`lifecycle-atlas-all/index.html`, 85 records / 15 plots); an
obsolete test ID that cannot map to production is recorded as an error, not
plotted as an unrelated default score.

Both lifecycle overview sheets and detailed final Layered, Creative v2,
Creative, counted and Pattern starts were inspected. The counted output has
two upper strokes followed by one full-device sweep, with no extra reversal
at the join. The generic syncopation outputs vary in character: the phrase
uses a repeated long/short timing motif, while the field and catalog choices
use broader pace/geometry development. These cases verify start intent and
valid shared-engine motion, not identical rhythmic interpretation across modes.
The planned-versus-quantized overlays track these outputs without added rests
or chatter; knot velocity and acceleration jumps round to zero in the detailed
plots. Lifecycle transition and Stop coverage comes from the separate
fake-transport tests, not these steady-loop figures.

## Local-model adaptation review

The local follow-up uses the available Gemma 12B Ollama model
`igorls/gemma-4-12B-it-qat-q4_0-unquantized-heretic:Q4_0`, explicitly allocating
16K context, reasoning off, and the ordinary 256-token utility reply allowance.
No installed conversation, external provider, engine or hardware is involved.
The captured baseline contains 40 lifecycle, negative-intent and Layered
composition cases. It follows intent in 30/40; its missed starts include valid
Creative/Pattern commands removed by the existing local word-based authority
checks. Questions, quoted instructions and prospective requests remain inert.

Do not assume that a change successful with GPT transfers to the smaller model.
The first combined candidate removed those start filters and used the hosted
start guidance, recovering four intended starts but introducing three false
starts on questions/hypotheticals. Its compact replacement Layered paragraph
also regressed fixed-width roaming and did not resolve full-device coverage.
That candidate was rejected, with no production authorization change. Both
`local-motion-baseline-production.json` and `local-motion-combined.json` remain
under the ignored evidence directory, including raw failed selections.

A narrower Layered example change and a same-sized clarification naming the
full-and-tip/base geometry choices were also rejected. The former lost a
previously correct fixed-width roaming response; the latter still missed
outer bounds and turned one regional request into a rejection plus another
generation. The final local planning prompt and its geometry examples remain
unchanged by this adaptation. The hosted phrase grammar is not enabled locally.

The adopted state-schema changes are small: local paused Creative advertises
only a reply, and interactive Pattern library uses the state-specific action
branches already tested with hosted chat. None/stop do not advertise target
fields; stopped start and running target are distinct. Existing local start
authorization remains in place. Hosted schema bytes and the autonomous base
schema are unchanged by this follow-up.

| Actual local production path | Intended results | Provider calls | Median generation |
| --- | ---: | ---: | ---: |
| Baseline, 40 cases | 30/40 | 41 | 818 ms |
| Final, same 40 cases | 31/40 | 40 | 803.5 ms |
| Final, 24 additional negative holdouts | 24/24 inert | 24 | 782.5 ms |

The final matrix uses the actual production service with no experimental prompt
or parser wrapper. The paused Creative case changes from two calls and an
unusable result (2.918 seconds) to one valid, inert explanation (1.125 seconds).
The median differences across the matrix are too small to establish a throughput
improvement. No extra normal-success generation is introduced. New holdouts
include commands inside quotations and negations, reported instructions,
future-only setup, and comparisons across all four motion modes.

Nine semantic misses remain in that deliberately difficult matrix: four legacy
starts removed by existing local authorization, one Layered request asking for
confirmation, two mixed full-device/regional requests that keep the old outer
bounds, and two regional pace-coordination requests. They are retained as
failures, not fixed by deterministic text rules or reported as successes. The
new correction path handles rejected output; it does not detect every valid
but inappropriate motion choice.

Repair evaluation is separate from initial response quality. Before this change,
local continuous modes rejected after the first invalid answer, while legacy
local chat already allowed one repair. The adopted continuous correction makes
one fresh generation from the unchanged original request, profile and contract
plus exact validator feedback. The rejected draft stays in diagnostics but is
not replayed to Gemma: replaying it anchored the model to invalid choices or
encouraged it to remove a requested control while claiming success.

| Local correction comparison, 768-token allowance | Control/intent checks | Median added generation |
| --- | ---: | ---: |
| Exact feedback with rejected draft | 6/8 | 928.5 ms |
| Compact feedback with rejected draft | 6/8 | 845.5 ms |
| Compact feedback, regenerate original request | 8/8 | 790 ms |

The final production helper at the ordinary **256-token** output cap passes
**14/14 structural and requested-control checks**, median **1.064 seconds**,
range **0.603–1.601 seconds** for the additional generation. Eight injected
failure fixtures cover malformed JSON, missing action, invalid enum/shape,
duplicate or incomplete groups, and compound requests. Six holdouts cover
questions, paused motion and long history in both modes. The two history cases
drop 13 older messages while retaining the exact latest request and unrequested
controls. Each fixture's initial rejection is injected; only its replacement
is live Gemma. This is recovery evidence, not the natural failure rate.

The compiled visual review adds an important qualification to the control
checks: `creative_ungrouped_focus` sets the full range and removes short-focus
mixing, but retains natural endpoint variation. Its strokes therefore do not
all traverse exactly 0–100 as requested. The original 14/14 control report is
retained unchanged; that result is not a claim of 14/14 physical-intent fidelity.
The separate `local-repair-compiled-assessment.json` measures an 88.44% mean
stroke and only 12.90%/11.29% of lower/upper turns within one point of the
base/tip. The unchanged 50% variation permits inward landings. Future live
evaluations now check compiled turn coverage separately from control fields.
No runtime language rule or forced parameter override conceals this limitation.

Full-profile budget checks pass all **24 configurations at 32K** (both modes,
four voices, 256/512/768 output limits). At 16K all six Utility combinations fit;
Explicit voice at 256 fits the initial request but lacks room for correction in
both modes. Some richer initial prompts already exceed that minimum. The helper
preflights the correction before another call and preserves the original failure
if essential context cannot fit. It never drops system/profile instructions or
the latest request to force admission. The default managed allocation remains
32K. These are budget checks, not additional live-generation reliability tests.

The local atlas at `local-atlas/index.html` contains **254 records, 32 distinct
plots and two overview sheets**, including rejected candidates and final
corrections. Both overview sheets were inspected, with detailed plots for the
false-start candidate, missing outer reach, valid full-length Layered correction,
the Creative v2 endpoint-variation miss, and long-history control preservation.
Layered full-length correction reaches both endpoints; the existing Creative v2
holds and variation remain visible rather than being smoothed away. Quantized
overlays follow the shared plan in these inspected outputs. No engine timing or
transport code is changed, and no hardware was used.

## Evaluation method and retained failures

Live tests use the installed protected ChatGPT sign-in in place, without copying
credentials. They invoke the production provider/parser/shared compiler with no
engine or transport. Final seam trials use a valid saved-style phrase with only
a pace layer, `gpt-6-sol`, reasoning off, 32K context and a 512-token output cap.
The alpha.69 comparison substitutes its original phrase instructions while
keeping the same provider, state and validator. It is a small prompt comparison,
not an estimate of general production reliability.

The old instructions produced 4/10 valid intended results; the other six were
invalid regional joins. Intermediate guidance removed all ten structural
errors, but one counted request gained an unnecessary second full sweep. The
explicit reuse-of-requested-travel instruction addressed that case. Repairs
are evaluated separately: their first rejected response is an injected retained
or deliberately malformed fixture, and only the corrective generation is live
GPT. This measures recovery, not how often GPT naturally makes those errors.

All trial reports, rejected JSON, latency logs and plots are retained under
ignored `.scratch/hosted-response-repair/`. Early experiments included an
overlong guide that exceeded a 32K essential-prompt budget, an overly strict
endpoint expectation, and hand-built starting scores with an inappropriate
center layer alongside a phrase. These are retained and excluded from the
definitive valid-starting-score comparison. The final fixtures assert their
starting score is valid. Compaction resolved the prompt-budget issue without
weakening context admission or increasing local prompts.

The earlier 12-case reach/count regression passed 12/12 with no repair or
fallback, median 5.172 seconds (3.584–9.700 seconds). It covers base reach,
double/triple counts, mixed regions, slower full sweeps in both directions,
explicit narrow and restricted bands, uncounted development and a question
that leaves motion unchanged. The later added center-timing guidance is checked
by the final regional and repair trials.

Final reports `joins-compiled-intent.json` and `repair-compiled-intent.json`
include the stronger compiled-turn check:

| Evaluation | Valid and intended | Calls per case | Median time | Range |
| --- | ---: | --- | ---: | ---: |
| Alpha.69 phrase guide, valid starting score | 4/10 | One real GPT call | 4.984 s | 4.227–8.022 s |
| Final phrase/region guide, valid starting score | 10/10 | One real GPT call | 5.492 s | 4.521–6.490 s |
| Final automatic/manual correction | 10/10 | One injected rejection + one real GPT correction | 5.005 s | 4.234–7.462 s |

The repair cases cover an invalid regional seam, duplicate operation keys,
invalid count, a counted connecting seam, and conflicting shape composition.
Automatic and manual paths each exercise all five. Times for correction are
the additional corrective generation, not the total latency of two real model
calls. The samples are small and do not establish a model-wide success rate.

## Motion plots

The combined atlas includes every intermediate and final regional/repair trial
and the reach regression: **226 records, 84 distinct plots, six overview
sheets**, generated by the shared compiler with no transport. It lives at
`.scratch/hosted-response-repair/atlas-complete/index.html`; inputs and exact
JSON remain alongside it. All six overviews were inspected, plus detailed
counted joins, regional pace directions, representative 32–100% outputs,
replacement proposals and quality outliers. The buffered whole-percent overlay
tracks the planned curves without added holds or reversal chatter in these
inspected outputs. Across the 143 moving records, the largest polynomial-knot
jumps are below 0.000005%/s in velocity and 0.00005%/s² in acceleration, including
the loop (zero at the plots' displayed precision).

In particular, record 140's extra full sweep was a valid structural result but
an intent failure. Record 196 passed the earlier, insufficient field-selection
test while its plot exposed missing lower-region strokes; that earlier report
is retained as originally captured, not reclassified as a final success. The
final equivalent records 216 and 226 have actual short strokes in both regions.
Record 201 retains four upper and two lower strokes with one connecting climb.
These observations motivated the two final guidance refinements and the
compiled-turn check; they do not justify changing the sampler or validator.

## Validation and limits

The frontend fixture matrix verifies manual and automatic repair, a second
invalid response, refusal, HTTP failure, and Stop during the second call.
It observes exactly two provider calls for a correction and no further retry;
refusal and network failures use one. The repaired reply uses the original user
row. Keyboard focus moves from the action to progress and back to the composer.
The simulator is stopped after each motion scenario. Fixture connections are
removed before restoring the review app to a working real local model.

Backend tests cover the unchanged contract, one-call valid replies, local
request equality, rejected/accepted evidence, single-use and expiry, original
stream cleanup races, context-budget rejection, refusal/fallback boundaries,
controller/session/credential/model changes, engine retarget races, video motion
ownership and Stop. Frontend tests cover the setting, repair SSE states,
same-session history refreshes, stale action removal, focus and localization.

The repair implementation passes the full Go suite, `go vet`, full
`golangci-lint` with zero issues, the `CGO_ENABLED=0` release build, and reachable
vulnerability analysis. The frontend passes 121 files / 946 tests, typecheck,
the five-locale audit and canonical production build. Linux CI supplies the
mandatory race suite: the available Windows clang targets MSVC and cannot run
Go's GNU-style race build. No gate is removed or weakened.

The isolated review app uses a real available Ollama model, with no copied
credentials, no fixture providers, LLM motion Off, Autopilot inactive and motion
idle. `scripts/check-review-llm.ps1` completes real generation. A text-only
request through the final app chat path returns a nonempty response with one
provider call, no repair or fallback, and motion idle. It takes 395 ms with a
139 ms first token and 3 ms model load. These are readiness observations,
not a local performance benchmark.

No sampler, motion timing, transport protocol or Autopilot decision policy
changes. Local schema/correction adaptations are limited to those evaluated
above. The separate fresh-start request updates compact interactive lifecycle
wording, including local continuous modes. Physical feel was not tested in
this investigation. Compiled plots and simulated dispatch validate the proposed
motion and control lifecycle; they are not carriage telemetry. A successful
validator cannot prove subjective motion quality or guarantee every future
model answer will follow intent.
