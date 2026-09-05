# ADR 0022: Conversation-first Lab sessions and retired legacy built-ins

- Date: 2026-09-05
- Status: Implemented for review; no release or merge authorized
- Supersedes: ADR 0020's manual legacy playback and preview-only LLM Lab behavior

## Context

The Lab had become a proposal inspector with substantial permanent documentation.
The user needs a normal conversation for testing new motion schemas, prompts
and features, including autonomous continuations, plus a simpler Help surface.
Tagging legacy patterns as deprecated excluded them from model curation but
preserved their enabled database state and manual playback after an upgrade.

## Decision

LLM Lab centers on chat. One title-bar test-mode list chooses the experimental
contract. Live motion and Autopilot are independent explicit session options.
Advanced model/prompt settings and plotted output are collapsed; detailed
instructions move to a Help tab with contextual topic links.

The server owns session configuration, history, score, inference scheduling,
and status. Lab Autopilot schedules only LLM requests using the selected
contract. Production Autopilot's planner and fallback policy remain separate,
so those policies cannot conceal a failed experimental contract. User messages
preempt automatic inference. A failed automatic reply pauses the scheduler.
Automatic proposals cannot raise speed or widen the current requested band.

The model receives current numeric planning limits, not a Handy version name
or the full motion-settings object. The engine derives the profile velocity
reference using its existing calibration. Semantic coordinates stay 0–100;
physical stroke-window mapping stays in the backend. The reference is labeled
as profile-derived rather than measured. Each turn refreshes these values;
trial exports retain full settings for reproducibility.

All motion continues through FlowTarget and the shared engine. Start uses the
existing Stop admission epoch. Accepted live replies use conditional retargeting
against a captured plan ID, checked together with the running epoch under the
engine lock. Startup, replacement, controller takeover, global Stop, disablement
and shutdown invalidate pending work. Failed transport application is distinct
from a valid model proposal in the conversation record. There is no new sampler,
transport payload, device loop or runtime dependency.

The continuous catalog grows from 10 to 17 distinct recipes. Seeding performs
an idempotent legacy retirement migration after preference promotion, forcing
all 81 deprecated built-ins disabled. Library resolution, enablement edits,
feedback undo, and manual motion entry points cannot re-enable or select them.
Their exports, customized names and weights survive. User content and saved
continuous-recipe preferences remain intact. This overrides the earlier
compatibility decision to keep legacy manual playback available.

## Consequences and validation

A backend test can continue while the user switches Lab tabs. Stop explicitly
ends it; the frontend renders its status rather than running its own scheduler.
Lightweight status polling avoids copying full prompts and history repeatedly.

Small-model errors remain observable. The revised selection-first prompt helps
Granite in the measured set, but does not establish general mapping reliability.
The 17 recipes, every accepted evaluated output, and wrong selections are
rendered from the shared engine. Physical comfort remains unmeasured for this
change. The three-zone tour's transitions are a visible high-jerk outlier within
the existing limits and should receive distinct physical feedback.

See [Lab workflow](../labs-workspace.md), [evaluation](
../lab-conversation-review-2026-09-05.md), and the mandatory lifecycle, migration,
conditional-target, parser, frontend and full repository checks.
