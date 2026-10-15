# Native Handy Bluetooth implementation review — 2026-10-14

This records the initial software review. The subsequent authorized
[hardware connection check](native-handy-hardware-review-2026-10-14.md) verifies
connection and Stop on an original Handy and records a Windows reconnect blocker.

The [integration plan](native-handy-bluetooth-plan-2026-10-14.md) was recorded
before runtime changes. [ADR 0046](decisions/0046-shared-native-bluetooth-worker.md)
records the resulting boundaries. This is a follow-up to the COYOTE/device-session
implementation in PR #319, on `codex/native-handy-bluetooth`.

## Implemented behavior

- Native Bluetooth is a separate Handy protocol in Setup, Settings and the
  connection manager's existing title-row menu. The small + Add flow is retained.
  Wording identifies the radio as belonging to the computer running MagicHandy.
- Selection saves without scanning or connecting. Explicit Connect uses the
  native helper; Disconnect stops/closes it. A remembered address is private,
  never silently replaced by scan fallback, and can be forgotten after disconnect.
  Multiple matching first-scan candidates produce an actionable error.
- One shared executable, one worker per connection, version-3 capability/profile
  handshake. The core remains pure Go. The native radio package is confined to
  the worker executable and enforced by architecture tests.
- Native and browser Handy transports share HSP validation, quantization,
  reversal, stream identifiers and tails. The core driver writes complete RPC
  frames sized to the negotiated ATT payload budget. It never splits protobuf
  bytes arbitrarily. Only supported WinRT write modes are selected before writing;
  a possibly delivered motion command is never retried in another mode.
- Optional clock sampling preserves write-only firmware support. Without usable
  samples, Play omits server time; successful native completion remains a command
  acknowledgment rather than a claim of physical position or Stop feedback.
- Stop fences unsent chunks while giving an already-submitted healthy write its
  bounded deadline. A hung write retires its worker. Late connections and stale
  transport instances cannot command or interrupt replacement sessions. Linked
  failures use the existing stop-all/continue-healthy policy; reconnect never
  rejoins a run automatically.

## Software scenarios and evidence

No real Bluetooth scan, pairing, connection or hardware motion occurred.
The scenarios below use in-memory links or real child processes with `-simulate`.

| Scenario | Result |
| --- | --- |
| Browser/native golden RPC fixtures | Same bytes for Stop, Setup, stroke fractions, 0–100 points, uint32 boundaries, Play timing and signed clock offset |
| 64-byte negotiated payload, 73 points | Complete bounded frames, exact point preservation and monotonically correct absolute chunk tails |
| Write-only firmware | Initialization stays usable after optional clock timeout; Play does not wait for a firmware reply and omits unsynchronized server time |
| Stop or caller cancellation during a blocked Add | Only the submitted chunk completes; remaining chunks are rejected and a healthy link still accepts Stop |
| Late firmware error with no message text | The correlated code-only rejection retires the connection |
| Native connect completes after Stop | Link closes without sending device commands or reporting ready |
| Disconnect deadline expires during connect | The connection slot stays reserved until the canceled attempt finishes; reconnect cannot overlap teardown |
| Old transport after reconnection | Cannot write to the replacement link |
| Handy worker killed beside a COYOTE worker | Handy loss is reported; COYOTE remains responsive |
| Native Handy failure in linked playback | Stop-all ends the run; continue-healthy retires Handy and keeps COYOTE running |
| Prepared native Handy excluded from the COYOTE engine | Global Stop still reaches Handy without removing its saved/connected state |
| Protocol switch and host access | Switching closes the native owner; uncontrolled requests are denied; public snapshots and diagnostic exports omit remembered hardware addresses, including error text |

The full Go suite and full race suite pass. `go vet`, `golangci-lint` (zero
issues), architecture checks, and the motion/transport/worker/session goleak
checks pass. The pure-Go core and worker build for Windows and Linux amd64.
Installer tests pass under PowerShell 5.1 and 7. All 900 frontend tests in 117
files, TypeScript,
five-locale audit and production build pass. The existing main-bundle size
advisory is unchanged in severity. See [budget measurements](goal-scorecard.md).

The bounded Handy decoder completed 1,118,099 fuzz executions in five seconds
without failure. A separate shared-engine run used the actual native worker
executable with `-simulate`, a 75% stroke for two seconds, and Stop: all seven
transport results succeeded, eight trace rows were exported, and the engine
ended stopped. Dispatch p50/p95/max were all 0 ms at the trace's integer
millisecond resolution; these are local simulation/IPC observations, not radio
or physical Stop latency. `native-worker-sim-trace.json` and its summary remain
in the ignored evidence directory. The isolated app's complete diagnostic ZIP
was also downloaded and checked for native transport status.

Browser review covered Setup's four protocol choices, Settings persistence,
the connection manager and a 390 x 844 layout. Selection did not connect.
Connect/Disconnect was exercised only against the isolated app's explicitly
reported simulated Handy; no playback was started through the UI. Emergency
Stop remained visible and the panel had no horizontal overflow.

The final isolated app at `http://127.0.0.1:50234/#/settings/device` uses a local
Ollama Gemma model. `scripts/check-review-llm.ps1` completed an actual generation
against that exact app's selected model. It is left disconnected with the native
protocol selected. Existing user-launched app processes/data were preserved.

Local, ignored evidence is under `.scratch/native-handy-plan/`: normal and race
logs, lint/vet, frontend build/tests, installer results, bounded decoder fuzzing,
startup/size measurements and UI screenshots. Committed golden vectors are in
`internal/handyble/testdata/browser-wire-vectors.json` and are verified by both
the Go and browser test suites. No user chat, credential or hardware identifier
was added to this review or committed fixtures.

## Intentionally unchanged

Shared engine sampling, motion curves, Handy speed limits, HSP buffer lead,
LLM prompts and context budgets, local-model selection, COYOTE strength ceilings
and user ownership, browser-local emergency Stop, Cloud REST and Intiface remain
on their existing paths. No native-only motion generator or background planner
is introduced. Golden wire parity establishes command mapping, not physical feel.

## Physical acceptance still required

The native implementation targets firmware-4 Handy RPC on Windows/WinRT and
Linux/BlueZ. macOS and firmware-3 Protocomm are outside this implementation.
Original Handy and Handy 2 advertising the required service still need actual
device/firmware matrix testing.

After separate authorization, capture firmware, OS/adapter and selected protocol;
then check discovery, remembered-device behavior, ambiguous scans, first Stop,
stroke window, bounded HSP playback, direction reversal, pause and global Stop.
Measure p50/p95/max dispatch and physical Stop latency, starvation/underruns,
long-run continuity and dual-device alignment. Export the diagnostic report
and motion trace for each run, and compare the same engine-generated patterns
against the browser path before changing any buffer budgets.

Explicitly test core crash, worker crash/hang, radio loss and both linked-session
policies. The last-will Stop is best effort: Windows kill-on-parent-exit and lost
radio can prevent delivery. Handy may continue previously buffered points until
they are exhausted; looping is disabled and pause-on-starvation is enabled.
Simulation does not establish physical failure decay, comfort, maximum speed or
radio latency. There is no claim here that native BLE is faster or physically
validated yet.
