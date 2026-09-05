# Optional Labs workspace

Every release includes Labs, disabled by default. Enable **Settings > General >
Labs > Enable Labs**. The setting applies immediately to all clients of this
app instance, persists in its database, and requires the active controller.
No special build or restart is needed. The Labs code and CSS load on demand;
`web/dist` remains the single embedded UI. Older executables need an update to
receive this setting. See ADRs 0021 and 0022.

## Workspaces

| Tab | Route | Purpose |
| --- | --- | --- |
| LLM Lab | `#/labs/chat` | Live conversation with a selected motion schema and prompt; optional live motion and Autopilot. |
| Motion Lab | `#/labs/motion` | Edit and compare shared-engine scores, inspect output, and start selected tests. |
| Guided tests | `#/labs/tests` | Follow saved rounds, rate each result, and add comments. |
| Observations | `#/labs/observations` | Save, inspect, export, or explicitly reuse review evidence. |
| Help | `#/labs/help` | Task-oriented documentation, with direct links beside related controls. |

Previous Settings Lab bookmarks redirect to their matching workspace. Visited
chat and motion editors stay mounted across Lab tabs to preserve drafts.
Leaving Labs discards frontend drafts and cancels an in-flight manual request;
an explicitly started backend test session continues until Stop, disablement,
replacement, or app shutdown. Its state is visible again on returning.
Emergency Stop remains mounted regardless of the route or controller status.

## Conversation testing

The title bar's **Test mode** selects the response contract and default prompt:
Creative v2, Layered, relative and layer edits, direct controls, ordered sections, simultaneous layers,
or the catalog using action names, descriptive IDs, or opaque handles. The three
catalog interfaces use identical motion content. **Configure** contains the
model override, schema constraint, prompt editor and Autopilot interval.
Configuration is held fixed while a test session is active.

**Creative v2** tests the same native stroke contract as its main-chat mode.
Ask for a focus location and width, local/full mixing, a fast direction with a
slower return, shrinking rebounds, inertia or variation. Follow-up edits retain
unmentioned groups. Inertia shifts the velocity crest within a stroke; it does
not mean force or measured ball physics. There are no runtime named presets.
Changing into or out of this mode starts a new compatible Lab score through
the backend. See [the Creative v2 review](creative-v2-motion-review-2026-09-05.md).

**Layered** tests the same contract as the production chat mode. It edits one
persistent score: range changes width, center changes location, and pace changes
travel rate. Unmentioned controls and layer attributes survive each edit. Named
geometry edits make coupled requests explicit; a separate relative timing field
distinguishes changing a period *by* a value from setting it *to* that value.
Turn softness is not offered to this model contract. See [the Layered review](
layered-motion-review-2026-09-05.md) and [ADR 0023](
decisions/0023-persistent-layered-motion.md).

Each inference request receives the current score, saved numeric speed bounds,
and the engine's semantic coordinate range and profile-derived peak velocity
ceiling. Device version names and unrelated motion settings are excluded from
model input. The envelope is a planning reference, not carriage telemetry;
calibration and physical stroke-window mapping remain backend responsibilities.
These values refresh from settings for each turn. The full settings still live
in exported trials so a reviewer can reproduce the compiled output.

Type and send messages normally. Without a session, accepted replies update the
backend score and its optional **Motion output** plot. Enable **Live motion**
and press **Start test** to start the score through the shared engine; later
accepted changes retarget that same run automatically. No per-reply audition
is required. Stop ends the test and cancels pending work. A plain Stop message
also bypasses inference. Simulation and unavailable transport remain explicit.
The main production motion mode can remain Off while Lab contracts are tested.

Enable **Autopilot** with or without Live motion. After a quiet interval (20
seconds by default; configurable 5–120), the backend requests a continuation
with the same model, prompt, schema, current score and matching conversation.
Layered and Creative v2 add a random delay of up to half the quiet interval, always respecting
the configured minimum. It starts with a fresh variation seed and can refresh
that seed on continuation without replacing the requested geometry or pace.
Drift and unequal smooth dwell times vary the motion inside those constraints.
Each score still has a finite repeat period; fresh realizations require an
accepted evolution edit, normally from Autopilot. Explicit exact-repetition
requests take priority. Seeds are retained in exports for reproducible review.
A manual message cancels an in-flight automatic turn and restarts the quiet
interval. There is one inference request per turn, without repair or fallback.
Malformed output and automatic proposals that increase speed or widen the
current requested band pause Autopilot for inspection. Stop remains independent
of the provider and transport result. A live transport failure is shown beside
the accepted reply; a valid proposal does not imply it reached the device.

Lab Autopilot is an inference scheduler for experimental contracts. Production
Autopilot retains its own planning, speech and fallback policies. Neither calls
a private motion sampler. The Lab uses `FlowTarget`, the shared engine's
admitted Start, and conditional retargeting against the expected current plan.
A reply cannot overwrite a newer plan or restart a stopped run. Controller
handoff, global Stop, Labs disablement and shutdown cancel the session.

User/assistant bubbles and the composer dominate the page. Enter sends,
Shift+Enter adds a line, and IME composition does not send. Cancel generation
keeps the draft. Response details expose the exact raw output, changed fields,
model, timing and call count. Creating a guided test is available there rather
than occupying every reply. Status polling fetches the full conversation only
when revision, busy state or session configuration changes.

## Motion and catalog

There are 17 enabled-by-default continuous recipes. Seven additions cover
base/tip/centered irregular drift, soft full-length turnarounds, even-beat width
variation, a three-zone section tour, and a moving window with varying width.
Startup disables all 81 legacy built-in rows, including previously enabled
rows. They cannot be enabled or manually played through the library or manual
motion API. Export, saved names and weights remain available. User-authored
content and existing preferences for continuous recipes are preserved.

Motion Lab's **Test generator** selects continuous flow or a historical generator
reference. **Score examples** load arrangements for the continuous generator;
they are not another generator selector. Historical comparisons retain their
limited controls and remain explicit experiments. Plots show commanded
estimates, not carriage telemetry. See [the current visual and model review](
lab-conversation-review-2026-09-05.md) and [the visual review procedure](
motion-visual-review.md).

## Observations and feedback

The most recent 20 Lab turns and current score live in backend memory. New chat
or restart clears that conversation. Production chat history, persona prompts
and saved preferences are separate. Export the conversation to retain raw
responses and prompts.

Layered also retains the four latest human requests independently of automatic
replies, so long Autopilot exchanges do not displace the intended character.
This context follows the same session lifetime. Production Layered obtains its
human context from the existing retained chat; it creates no second history
store and does not import Lab conversations or observations automatically.

Observations, their captured sources, test sequences and submitted ratings or
comments persist in the app's SQLite database across restarts. The actual path
is shown in Observations, Guided tests and **Help > Storage and use**. Unsaved
comments are drafts in the current page only. Saving evidence does not train a
model, alter prompts, change preference weights, or change motion. **Use in
chat** creates an editable draft; only sending it supplies that evidence to the
selected model. Exports contain the record and its captured context.

Disabling Labs rejects all Lab APIs and cancels Lab requests and sessions. It
stops Lab-owned motion through the shared engine, leaving unrelated motion
alone. Completed in-memory turns and durable feedback remain available when
Labs is re-enabled. A late response cannot commit after disablement.
