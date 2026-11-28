# Edge and app review, November 27, 2026

Claude Opus 5.5 at Max effort and Gemini 4 Neon at xhigh effort reviewed
the recent Edge work and the application independently. Each received the
tracked source at `a51f5ecd` and a request to examine the motion engine,
transport receipts, SQLite history, provider routing, background workers,
accessibility and the build. Follow-up reviews checked the reported races
and the proposed fixes. The reviewers received no installed conversations,
connection keys or runtime files.

## Edge controls

The Edge branch rechecks the running plan before a repeated press can
cancel other work. A late status poll cannot erase a newly installed
hold's Return control. A linked transport needs a positive receipt of the
requested kind from every participating owner. A queued write superseded
by Stop leaves the device eligible for later playback. A failed entry
relinquishes scheduling. Recovery on the old plan requires an admission
or cancellation error and unchanged ownership. An unconfirmed delivery
blocks recovery Start, and an installed hold still has a manual Return.

Only Advanced with reasoning receives previous-hold feedback. The planner
checks committed conversation revisions, including deletions, before
applying a delayed decision. Each eligible chat keeps two moving or
motor-tempo observations and one still pause. An intervening pause keeps
the earlier moving hold's feedback available.

A reasoning-enabled loopback Gemma 4 12B produced zero complete plans in
six trials at 192 output tokens and six at 1,024. Both exact requests
passed on each run. Both close-call requests extended the hold. One kept
the same return pace, so five of six passed the stricter intent check.
Median generation was 2.49 seconds at the old limit and 8.34 seconds with
a completed plan. The button applies the hold before inference. On the
signed-in Terra replay, “That was too close” changed the next proposal
from 76 seconds at 24% to 100 seconds at 20%. A fresh request for 42%
set the current pace to 42%. The shared engine rendered 47 distinct steady
trajectories and 140 bounded transitions. The overview and detailed curves
showed the same commanded shapes as the previous Edge export.
[Edge trials and visual review](edge-feedback-review.md)

## Long conversations

Validated routes keep archive reads separate from motion. Named and
opening lookups have distinct schemas, and Inkling retains its required
object root. The default Gemmas keep the union shape used in their earlier
context regression tests. Missing keys, invalid field types and mixed
operations yield a visible routing error. A complete read-only lookup with
unusable arguments explains that the search could not run and sets the
fallback diagnostic. The reader reserves “Unavailable” for a fact it could
not verify. A malformed router or mixed recall and motion
envelope cannot trigger an ordinary motion repair. First-token timing
ends at the first answer-model token. Total cost still includes
the router and all evidence calls.

| Live profile | Opening paces and following change | Named checkpoint and baseline | Missing evidence | Fresh motion |
| --- | --- | --- | --- | --- |
| Gemma 4 E4B, Simple | 25%, 35%, +5 points | juniper, 30% | Correct for partial and absent history | 42% |
| Gemma 4 12B, Intermediate | 25%, 35%, +5 points | juniper, 30% | Correct for partial and absent history | 42% |
| Signed-in GPT-6.1 Terra, Advanced with reasoning | 25%, 35%, +5 points | juniper, 30% | Correct for partial and absent history | 42% |

The replay used the production streaming handler, later 30% and 38%
distractors, and a simulated transport. The historical answers sent zero
device commands. Each model executed the separate 42% request without
repair or fallback.

## Devices and local workers

The VacuGlide simulator reproduced accepted tempo and empty appends while
stopping and after session loss. Refused appends now leave the previous
stream and revision intact. A gated simulated Bluetooth link exercised a
Coyote reconnect queued during Close. The owner closes admission while it
still owns the operation lock, sends zero, and drains the link and pacer.
These tests check software commands and acknowledgments. No physical device
was connected.

During release testing, the motion race suite exposed a queued dispatch
tick that could replace a sampler fault with “motion run was invalidated”
after the fault Stop. The loop now checks run ownership before recording
an error. A deterministic regression delivers the late tick after Stop
and after a replacement run takes ownership. Both preserve the original
diagnostic and issue zero device commands.

When hosted chat uses a separate local planner or refusal fallback, the
local editor reads that worker's status and targets it for Load and Unload.
Editing a local runtime drains and releases the old worker. A regression
test keeps a hosted generation admitted across the edit and completes it
through its original provider. Rebuilding the managed runtime holds local
admission through installation and reapplies the latest startup load
policy. A duplicate or invalid install request leaves the worker alone.

The review retained the existing stale-plan checks and mechanical speed
limits. A second local inference slot would need a measured prefix-cache
benefit and a memory budget on the supported GPUs. The existing single
slot served all three replay scenarios.
