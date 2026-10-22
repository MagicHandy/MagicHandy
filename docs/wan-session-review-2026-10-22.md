# WAN session review — October 22, 2026

## Scope and evidence

Reviewed the installed **0.1.0-alpha.67**, commit `25353690`, and based this
branch on that release. The controller used the full application over WAN and
reported keeping the browser visible while idle. Read-only examination covered
the selected conversation, account/control audit, current settings and the
downloaded diagnostic trace. No device connection, takeover, playback or Stop
was issued during the review. User chat, account identities, credentials and raw
diagnostics are not included in the repository.

## Findings

- One login lost control at the old 30-minute idle boundary. Passive viewing,
  heartbeats and ongoing motion intentionally do not count as login activity.
- Seven controller heartbeat losses appear during the retained session window;
  two interrupted active chat motion. These are separate from idle sign-out.
  The server retained no client-side heartbeat timing, so this is not proof of
  the exact network/browser cause for each interruption.
- The active-controller retry loop can itself exhaust its 15-second lease:
  after two three-second request timeouts, four/eight-second backoff delays
  push the next attempt beyond expiry. A deterministic fault-injection test
  reproduces this scheduling defect without hardware.
- Chat aborted its admitted stream whenever an independent status channel made
  the UI read-only. A short observation failure could therefore discard a valid
  model turn despite a still-valid server lease. Two user turns have no committed
  assistant reply or retained model failure; this defect is consistent with
  those gaps, but the retained records cannot establish their exact cause.
- Nine successful model requests used ChatGPT `gpt-6-sol`, one provider call
  each, without repair/fallback. Latency ranged **2.9–13.3 seconds**, median
  **5.9 seconds**. The slowest spent **10.7 seconds** before first provider
  activity; preparation was **1 ms**. No model request reached the configured
  120-second timeout in these successful records.
- Retained chat starts added **6.3, 10.3 and 11.2 seconds** between model
  completion and Cloud Play. Running updates queued handoffs **1.6–2.4 seconds**
  ahead. Cloud append requests had median **324 ms**, p95 **373 ms** and maximum
  **756 ms** over 383 successful retained calls. Startup positioning, state
  verification and prebuffering explain substantial non-model delay; those
  safety steps are unchanged.

## Motion assessment

Rendered the retained shared-engine samples actually sent to the Cloud transport,
including transitions, for all nine distinct pattern/pace groups. Inspected the
overview and detailed tip-region, alternating-region and final base-region plots.
These are commanded samples, not measured physical position. The ring retained
512 rows and dropped its first 92; the earliest start cannot be reconstructed.
One failed Autopilot startup and the two unanswered chat turns remain recorded
as failures/gaps rather than invented successful trajectories.

- The earlier Creative v2 output repeatedly returned to the base with varying
  reach. The later gentle Layered start likewise returned to the base, with a
  broad, slowly varying excursion rather than a small fixed local massage.
- The tip-focused update settled into short strokes at approximately 70–100%
  position. Its 22% pace matched the configured ceiling at that stage.
- The compound region/tempo request produced repeating upper/lower groups and
  pauses at an overall 12% pace. Spatial alternation worked; distinct fast and
  slow phases were less faithful. The follow-up
  [hosted pace review](hosted-pace-review-2026-10-22.md) tests and addresses that
  planning/representation gap.
- The full-range request produced regular 0–100% sweeps at 22%, with rounded
  reversals. The final gentle request settled into 0–30% strokes at 7% after
  the queued prior sweep/transition cleared.

The WAN fixes leave motion generation and dispatch unchanged. The subsequent
[pace/compound-motion changes](hosted-pace-review-2026-10-22.md) extend hosted
planning and optional Flow modulation; existing scores, speed mapping, limits,
sampling and startup retain their behavior. No deterministic language rules were added.
Private artifacts remain under `.scratch/wan-session-review/`: the diagnostic
inspection scripts, `motion-overview.png`, nine detailed plots, and test logs.

## Changes and validation

Login idle sign-out defaults to eight hours and can be changed or disabled under
Access → Sessions. Updates apply to still-valid logins; expired credentials are
retired transactionally before extending a policy. Absolute 12-hour expiry,
revocation, grant checks, foreground-only renewal and the 15-second controller
deadman remain enforced. The reduced remote portal cannot access host policy.

An active controller keeps the normal two-second retry cadence after transient
failures. Discovery/observers retain exponential backoff and each channel still
has one bounded request. Already-admitted protected chat survives observation
staleness; new input remains locked, and server-side fences plus Stop, confirmed
ownership/generation/epoch changes, hidden documents and teardown still cancel it.

Regression coverage exercises old-policy upgrade, persistence, disabled idle
expiry with absolute/revocation checks, live-policy shortening and extension,
expired-session non-revival, administrator/remote boundaries, two WAN heartbeat
timeouts and pending-chat cancellation boundaries. Local `go test ./...`,
`go vet ./...`, `golangci-lint` (zero issues), frontend typecheck, all **929**
component tests and five-locale validation passed. The canonical frontend and
`CGO_ENABLED=0` Windows core build passed. Race-detector validation runs in CI's
Linux environment; this Windows host has no configured MinGW compiler.

The isolated simulator at `http://127.0.0.1:50262/#/settings/access/sessions`
shows the new eight-hour default and an authenticated session list. It uses the
available local Gemma 12B through Ollama, with no copied user credentials.
`scripts/check-review-llm.ps1` completed a real generation; a text-only request
through the app chat returned a nonempty reply in **340 ms**, one provider call,
without repair, fallback or a motion command. The installed app was not
replaced. A real WAN repeat is still required to confirm the user's particular
network no longer produces interruptions.
