# Optional Labs workspace

The additional September 4 experiments add **Compare motion experiments** in
Motion Lab and Guided tests. It captures five rounds: continuous reference,
correlated drift, softer reversals, a steadier beat and their combination.
Each keeps its exact score and preview for ratings, comments and export.
Creating a sequence does not start motion. New controls apply to Continuous
flow; historical generators retain their applicable controls.

In LLM Lab, expand **Experiment setup → Control interface → Relative and
layer edits** to test relative changes or editing one layer while retaining
the others. Schema constraints turn on for this method. Inspect actual
changed fields in Response details: a valid response can still miss intent.
Model results and physical-feedback questions are documented in
[the experiment review](motion-ideas-review-2026-09-04.md).

Every release includes Labs, disabled by default. Enable it using **Settings >
General > Labs > Enable Labs**. The switch applies immediately and persists
in the app's settings database as `labs.enabled`; no restart or special build
is required. The choice applies to all clients of that app instance and only
the active controller may change it. An older installation with no Labs
setting starts with it disabled.

The four tabs separate the conversation, motion editing, guided testing and saved review
evidence. The workspace and its CSS are loaded only when an enabled Labs route
is opened. `web/dist` is the one canonical shipping UI for every build.
`npm run build:labs` and `scripts/build-labs.ps1` remain compatibility helpers
for this same release build. ADR 0021 supersedes the build-time exclusion in
ADRs 0019 and 0020. Previously published executables must be updated to gain
this setting.

Disabling Labs cancels lab requests, rejects new lab API calls and auditions,
and stops an active lab audition through the shared engine. Regular motion
continues. A late model reply cannot update a preview after disable, including
if Labs is immediately enabled again. Completed in-memory turns and saved
observations remain available. A disabled Labs bookmark offers the Settings
link instead of mounting the workspace. Emergency Stop remains available in
every state.

## Navigation and workflow

| Tab | Route | Primary workflow |
| --- | --- | --- |
| LLM Lab | `#/labs/chat` | Ask for a change, inspect the reply and backend preview, explicitly audition when ready. |
| Motion Lab | `#/labs/motion` | Edit pace, range and gradual variation; inspect a plotted estimate; optionally compare historical methods and start the selected test. |
| Guided tests | `#/labs/tests` | Follow a saved sequence, rate each round, add a comment and export the feedback with the captured results. |
| Observations | `#/labs/observations` | Read saved review evidence, inspect its captured source, export it or reuse it in a chat draft. |

The old `#/settings/llm-lab` and `#/settings/motion-lab` bookmarks redirect to
the matching tab. Settings contains the availability switch; the workspace
itself lives on the Labs page. Visited chat and
motion editors remain mounted while switching these tabs, preserving drafts,
experimental prompt edits and in-flight generation. They are discarded when
leaving Labs; a pending chat request is canceled on unmount or emergency Stop.
Completed turns and the current score remain backend-owned.

LLM Lab uses ordinary user/assistant bubbles and a composer anchored below the
scrolling conversation. Enter sends; Shift+Enter adds a line, and IME
composition does not send. Cancel generation keeps the draft. Response details
show the original output, changed fields, model, call count and latency.
Experiment setup contains the model override, control interface, recipe naming,
schema option and editable prompt. Changing the interface loads its default
prompt; subsequent requests use matching prompt/interface history.

The current preview is a backend estimate. Plotted position and effective pace
lead; full-phrase metrics, velocity and baseline overlays live under Compare
methods and dynamics. The comparison table is read-only. Test generator in the
controls panel is the sole generator selector. Continuous flow plays the
continuous score; its Score examples buttons load a single section, layered
score or section sequence without switching generators. The preview names
both generator and arrangement, and Test target repeats what Start will play.

Creative baseline and Anchored range are historical generators. They share
only pace, outer band, shortest span and seed with the continuous score;
Anchored range also applies its anchor. Both use the fixed wander profile,
30% variation and a minimum span of 20. Continuous layers, sections, range
memory and pace variation do not drive them, so those editors are hidden when
viewing a historical generator. Creative fixes the anchor at 50, shown as a
disabled control. Returning to Continuous flow restores its arrangement.

No curve is synthesized in the UI. Chat replies, slider
edits, saved observations and tab changes do not start motion. An explicit
audition still uses the shared engine, saved-limit fingerprint, controller
lease and Stop admission checks. The global Stop remains mounted.

## Guided test sequences

**Start a motion feel check** creates a short comparison of Creative baseline,
Anchored range and Continuous flow using the current backend lab score. A band
under 20% supports only the continuous generator and creates one round. In
Motion Lab, **Create test sequence** captures the displayed score for the same
comparison. Beside an LLM reply, it creates two rounds: the motion before the
request, then the reply and its resulting motion. A rejected reply remains a
testable response with its raw output and failure; it has no audition.

Each round presents one instruction and the captured engine preview. Choose
**Needs work**, **Mixed** or **Good**, specify whether you reviewed a visual
preview, a device/simulated audition or an LLM reply, then optionally add a
comment. **Save and next** persists the answer before advancing. **Skip this
round** records a skip and retains the comment. The progress bar counts saved
rounds, and completion reports reviewed and skipped counts separately; a
positive rating never earns more progress than reporting a problem. The
review basis is explicitly self-reported, not inferred physical telemetry.

An audition remains optional, requires the controller, and uses the existing
semantic motion start path with the captured limit fingerprint. It repeats
until Stop. The UI and backend reject advancement while the engine is running
or paused. Creating a sequence, saving an answer and opening the next round
never issue motion. Emergency Stop remains mounted. If saved limits or the
compiled preview have changed, an old capture can still receive visual/text
feedback, but its UI audition is disabled; create a new sequence to test the
updated output. No new motion generator or private playback loop is involved.

Saved runs resume at their first unanswered round after navigation or restart.
Answers and their original samples, score, limits, build version/commit and
LLM trial are stored together in this instance's `magichandy.db`, under
`app_kv` key `labs.test_runs.v1`. **Where feedback is saved** shows the exact
database path. Storage is capped at 20 sequences, 12 rounds per sequence and
16 MiB total, with no automatic eviction. Unsaved form edits last only while
their round remains on the page. **Export feedback** downloads a JSON report
including captured outputs and all saved answers, even for an unfinished run.
Deleting a sequence requires confirmation. Disabling Labs preserves the runs.
Guided feedback is separate from freeform Observations; neither is inserted
automatically into prompts, production preferences or model training.

For a prepared change review, the controller can create an authored sequence
through `POST /api/labs/tests`: supply a `title` and 1–12 `steps`, each with a
`title`, `instruction` and `target` in the existing observation-target shape.
Motion targets include `source: "motion"`, `method`, `spec` and the current
`settings_key`. LLM targets include `source: "llm"`, `revision` and
`turn_index`; optional `phase: "before"` or `"after"` selects the score.
The backend captures and validates these references before saving anything.
Alternatively supply `preset: "motion_comparison"` or `"llm_comparison"`
and an optional `target`. Custom steps and presets cannot be mixed.
Creation returns the run ID; open `#/labs/tests/<id>` for the user to follow.

`GET /api/labs/tests` lists progress without returning every sample;
`GET /api/labs/tests/<id>` returns the captured tests and feedback for review.
`POST /api/labs/tests/<id>/feedback` accepts `revision`, `step_id`, `rating`
(1–3), `basis` and `comment`; use rating 0 with basis `skipped` for a skip.
Only the current step at the matching revision is accepted. Transactions
reject duplicate or stale submissions rather than advancing multiple rounds.
These routes require Labs to be enabled, and mutations require the controller.

## What is saved, and what it affects

| Data | Lifetime and location | Effect |
| --- | --- | --- |
| Lab conversation and current score | Latest 20 turns in backend memory. New chat or app restart clears them. | Accepted replies update only the lab preview. Export conversation downloads the available trials, including prompts and raw outputs. |
| Unsent messages, edited prompts and manual preview drafts | Browser component state while Labs remains open. | No model request or motion until the corresponding explicit action. |
| Observation draft | Browser component state until Save, close, or leaving Labs. | No durable record exists until the app confirms Save. |
| Saved observation | This app instance’s `magichandy.db`, existing `app_kv` key `labs.observations.v1`. The UI displays the exact absolute path. Survives new chats and restarts. | Review evidence only. It is never read automatically by model prompts, production chat, preferences, training or motion selection. |
| Observation export | JSON file downloaded explicitly by the user. | Includes the observation and captured source; it does not apply changes. |
| Guided test sequences and feedback | This instance's `magichandy.db`, key `labs.test_runs.v1`; survives app restart. | Captured review evidence and saved progress only. Available in Guided tests and JSON exports; no automatic prompt or motion changes. |

Observe preview captures the selected method, score and saved motion limits.
The attachment stays on that captured preview even if sliders are edited later.
Observe reply captures the selected trial: request, reply, raw output, prompt,
model, method, schema setting, before/after scores and the original trial limits.
A save with a stale conversation revision is rejected and its draft is retained.
This prevents a reply shifted out of the bounded history from being mistaken
for another reply. Saving a trial after a limits edit retains the original
limits, not the newly edited ones.

Saved records are capped at 200 and 4 MiB of JSON, with transactional writes.
Reaching a bound reports a failure and keeps existing evidence; there is no
silent eviction. Save and Delete require the controller. Reads are available
to authorized readers and use `Cache-Control: no-store`. Deletion requires a
second explicit UI action. Observations are local to the selected data directory;
another app instance with a different directory has a different collection.

Use in chat copies the observation and a source description into the composer.
It neither sends a request nor changes a prompt. Edit the draft to explain the
next test, then Send when desired. Old “Observation notes” fields were only
browser state included in Export comparison. They were never saved in the app,
so the new collection cannot recover them; retained exports remain the evidence.

## Review evidence

The September 4 guided-test review used the standard release build in an
isolated instance at localhost:49844. The complete three-round motion sequence
was followed through the visible UI: visual feedback on the baseline, a skip
with a retained comment on Anchored range, and visual feedback on Continuous
flow. The completion summary correctly showed two reviews and one skip.
Every captured generator was inspected. These were agent visual QA records,
not user reports of physical feel; they were exported to ignored evidence
files and removed from the review collection before leaving a fresh sequence.

A live Gemma 12B request changed only `anchor_percent` to hold the tip
(596 ms, one provider call). Create test sequence captured the before and
after curves with the request, reply, raw output and limits. The saved run
remained identical after a real restart, resuming at round two despite the
temporary LLM conversation being empty. Visual review also led to separating
the changed reply from the before-round card. Final text-only production chat
returned in 375 ms without repair, semantic fallback or motion; the provider
readiness script passed on the final app. The motion trace export contains no
dispatches. The review instance has no device credentials, and other app
instances were preserved.

The full Go suite and race suite passed, followed by race checks on the
changed packages after final refinements. Vet, lint, frontend type and
localization checks and 468 frontend tests passed. Regression tests cover
source capture, custom sequences, rejected LLM output, restart/resume,
duplicate and concurrent saves, step order, storage bounds, corrupt data,
controller/Labs gates, stale limits/output, comments retained on failed saves
and skips, and rejection of advancement while motion is running. Budget
measurements are in the goal scorecard. Ignored evidence uses the prefix
`.scratch/guided-tests-`, including the frozen QA runs, before/after restart
snapshots, live trial, text-chat SSE, readiness, traces and build/test logs.

The September 4 release-setting review used the standard build, without a Labs
build tag, in an isolated app at localhost:49843. A fresh database started with
Labs disabled. The visible General Settings switch enabled its sidebar entry
and workspace, and the choice survived a real app restart. The exact review
instance passed `scripts/check-review-llm.ps1` using local Ollama Gemma 12B.
Production text-only chat returned in 343 ms with one call, no repair,
semantic fallback or motion. A live LLM Lab request returned a valid reply in
3,195 ms with one call and no score changes; its normal chat layout was
inspected visually.

Regression tests cover default-off API and audition rejection, persistence,
unrelated settings updates, controller permissions, stopping an active lab
audition, preserving regular motion and saved observations, cancellation of
a blocked audition start, and rejecting a late model reply across disable and
re-enable. The complete Go suite and race suite, vet, lint, frontend type and
localization checks, 459 frontend tests, and the ordinary pure-Go build passed.
One initial concurrent validation run hit the existing Autopilot unsafe-start
retry assertion; the focused test then passed 30 repetitions and the full
suite passed on rerun, without changing that assertion or the mode code.
The footprint and fresh-data startup measurements are in the goal scorecard.
Evidence files use `.scratch/labs-release-` and `.scratch/labs-setting-`.
The review instance has no device credentials and issued no auditions; the
existing app instances remain running. This setting changes availability and
request lifecycle, not motion character or the LLM motion contract.

The September 4 workspace review used an isolated app at localhost:49842 and
the available local Ollama Gemma 12B model. A live request to hold the tip while
varying reach changed only the anchor in the accepted preview, with one provider
call. A real reply exposed an inherited avatar-grid layout bug; the lab now
uses a dedicated flex layout with normal wrapping, verified visually with a
fresh live response. The conversation composer remained visible throughout.

A saved LLM observation survived a real app restart and was visible with its
database path and captured source. Use in chat populated an unsent draft. Old
Settings bookmarks redirected to the new tabs. Backend tests cover concurrent
writes, stale references, reader guards, bounds, corrupt storage, reset/reopen,
original-limit capture and the absence of automatic model or motion effects.

The clarified generator selector was checked with a layered score, a switch
to Anchored range, and a switch back: only applicable controls were shown, the
audition target followed the selected generator, and the layered score was
retained. Both motion and LLM observations were saved in the review database.
The final live lab reply took 539 ms with one provider call and changed only
`anchor_percent`. The required provider readiness script and production
text-only chat path also passed (291 ms, one call, no repair/fallback/motion).
Frontend validation passed 455 tests; Go tests, race, vet and lint passed in
both public and development configurations. Binary and browser budgets are
recorded in the goal scorecard. Ignored evidence files use the prefix
`.scratch/labs-workspace-`, including the final live trial, text-chat SSE,
readiness result, test logs and review trace export.

The review instance has no copied device credentials and issued no auditions.
The original app on localhost:49841 remains running. This interface review
does not establish physical motion feel; use the existing visual atlas and
shared-engine device audition process for motion-character changes.
