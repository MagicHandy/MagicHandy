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
| LLM Lab | `#/labs/chat` | Live conversation with a selected motion schema and prompt; optional live motion and Autopilot; one request compared across modes. |
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

Mode buttons select the response contract and default prompt for the five
main modes: Creative v2, Layered, Stroke ends, Groove and accents, and Plain
words. **More modes** holds relative and layer edits, direct controls, ordered
sections, simultaneous layers, and the catalog using action names, descriptive
IDs, or opaque handles. The three catalog interfaces use identical motion
content. **Configure** contains the model override, schema constraint, prompt
editor, Autopilot interval and export. Changing the mode during a test restarts
the test in the new mode; the rest of the configuration is held fixed while a
test runs.

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

**Stroke ends**, **Groove and accents** and **Plain words** test possible
replacements for Layered. They edit one stroke score through three
vocabularies: turn positions as numbers, a groove with accents, or everyday
words for how deep every stroke goes and how far it pulls back. The score says
where every stroke bottoms out and turns back, in absolute slider positions,
with seeded variation and occasional accents, and compiles through the shared
engine like Creative v2. An invalid value rejects the whole reply. Changing
into or out of these modes starts a new Lab score. See [the stroke vocabulary
review](lab-stroke-modes-review-2026-09-26.md).

Each inference request receives the current score, saved numeric speed bounds,
and the engine's semantic coordinate range and profile-derived peak velocity
ceiling. Device version names and unrelated motion settings are excluded from
model input. The envelope is a planning reference, not carriage telemetry;
calibration and physical stroke-window mapping remain backend responsibilities.
These values refresh from settings for each turn. The full settings still live
in exported trials so a reviewer can reproduce the compiled output.

Type and send messages normally, or tap a quick request such as Deeper or Just
the tip. Without a session, accepted replies update the backend score and the
**Motion now** plot. Turning on **Live motion** starts the score through the
shared engine at once; later accepted changes retarget that same run
automatically. No per-reply audition is required. Stop ends the test and cancels pending work. A plain Stop message
also bypasses inference. Simulation and unavailable transport remain explicit.
The main production motion mode can remain Off while Lab contracts are tested.

Turn on **Autopilot** with or without Live motion; it also starts at once.
After a quiet interval (20 seconds by default; configurable 5–120), the backend
requests a continuation with the same model, prompt, schema, current score and
matching conversation. Layered, Creative v2 and the stroke modes add a random
delay of up to half the quiet interval, always respecting the configured
minimum. It starts with a fresh variation seed. Like production Autopilot, a
continuation in these modes reads the latest human lines,
keeps whatever still applies (including a wish to keep the motion exactly as
it is) and develops the rest within saved limits
([ADR 0031](decisions/0031-model-judged-autopilot-continuity.md)).
Drift and unequal smooth dwell times vary the motion inside those constraints.
Each score still has a finite repeat period; fresh realizations require an
accepted evolution edit, normally from Autopilot. Seeds are retained in exports
for reproducible review. A manual message cancels an in-flight automatic turn
and restarts the quiet interval. There is one inference request per turn,
without repair or fallback. Malformed output pauses Autopilot for inspection.
For the other experimental methods, automatic proposals that increase speed or
widen the current requested band also pause it. Stop remains independent
of the provider and transport result. A live transport failure is shown beside
the accepted reply; a valid proposal does not imply it reached the device.

Lab Autopilot is an inference scheduler for experimental contracts. Production
Autopilot retains its own planning, speech and fallback policies. Neither calls
a private motion sampler. The Lab uses `FlowTarget`, the shared engine's
admitted Start, and conditional retargeting against the expected current plan.
A reply cannot overwrite a newer plan or restart a stopped run. Controller
handoff, global Stop, Labs disablement and shutdown cancel the session.

The conversation and composer fill the main column. Each reply is a compact
entry with its outcome and mode; **Details** exposes the exact raw output,
changed fields, model, timing and call count, and creating a guided test is
available there. **Send again** repeats a message. Enter sends, Shift+Enter
adds a line, and IME composition does not send. Cancel generation keeps the
draft. Status polling fetches the full conversation only when revision, busy
state or session configuration changes.

**Compare modes** sends the typed message, or the last one sent, to each main
mode in turn, each from its own starting score. Every result shows the reply,
its outcome, the reach and a 12-second plotted estimate, and **Try this mode**
switches to that mode. A comparison is not saved, changes neither the
conversation nor the score, and never plays. It is unavailable while a reply is
generating or a test runs.

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
