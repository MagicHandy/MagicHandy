# Edge feedback belongs to the selected planning role

Accepted for the Windows Edge control update, November 27, 2026.

A button press has two separate effects. The shared engine immediately prepares
a moving or still hold. Autopilot can then ask its selected planner how long
a moving hold should last and how it should return. A person may describe
the previous hold, give an exact preference for the next one, or talk about
another subject while the device is running.

Record at most two mechanical button observations per chat in SQLite. Each
observation references the sequence of the last committed message at the press.
Store the type and requested pace after the engine accepts the action. An
accepted automatic plan adds its duration, return pace and style. Its duration
describes a plan. A manual Return can end the hold sooner.

The planner may read bounded, verbatim human remarks between the two presses
when its resolved capability is Advanced (with reasoning) and
its context policy allows the conversation. Initial reactions, recent references
to Edge or a hold, and the latest corrections can bring at most seven complete
user messages of up to 384 bytes each into the request. The model decides their
relevance. Message deletion and retention apply at read time. Deleting the chat
cascades to its button observations. Separate chats have separate evidence.

The capability applies to any model the user assigns to Advanced (with reasoning),
including a local model or a named connection. Signed-in GPT-6.1 Terra was
the default representative in the live evaluation. An Advanced conversation
role can describe the last button event even with a different planner. Its
prompt says whether that planner can use feedback on the next press. An Advanced
conversation-aware planner can use human remarks even with a lower-tier
conversation model. Technical-only planners, the Decisions service, Simple,
Intermediate and Advanced without reasoning keep their established planning
contracts. Storage contention can cost an observation. The mechanical action
and Stop remain available.

The engine uses the validated duration and return submitted by the planner.
A previous duration can recur. The duration window is 8–180 seconds and the
existing speed bounds apply. Planner errors, a second Edge press and newer
user control continue to revoke automatic return. Other models assigned to
Advanced can be evaluated using the same multi-turn and distractor cases.
[Evaluation](../edge-feedback-review.md)
