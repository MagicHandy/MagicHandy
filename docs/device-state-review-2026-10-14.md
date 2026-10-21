# Device state and linked-session review — 2026-10-14

Worktree: `MagicHandy-coyote`, branch `claude/coyote-estim`.
Decision: [ADR 0045](decisions/0045-device-inventory-and-linked-sessions.md).

## Implemented behavior

| Action | Saved inventory | Connection | Playback |
| --- | --- | --- | --- |
| Title-row + | Adds one missing device | Unchanged | Unchanged |
| Connect | Retained | Explicitly prepares device | Does not start or join |
| Use in playback | Retained | Retained | Selects next run; current run stops |
| Exclude | Retained | Retained; Coyote zeroed | Removes participant; current run stops |
| Disconnect | Retained | Closed | Stops the run if participating |
| Remove | Removes row, including last row | Closed | Excludes device |
| No devices | Empty list is retained | None | Chat remains available; engine stays idle |

Settings separates playback selection from saved connections and exposes
**On device failure**. The default stops the linked run when either participant
fails. **Continue on the healthy device** is explicit and applies only after
successful startup; the failed participant is retired from that run. A second
failure ends it. The backend publishes degraded/failed session status for the
manager. Reconnecting never rejoins automatically. Emergency Stop remains
unconditional, including prepared devices outside the active engine.
An already-open Settings form follows the manager's confirmed inventory,
playback and protocol changes while retaining unrelated unsaved edits. Saving
that form no longer restores a stale playback selection.

An engine's Coyote handle binds to its link. Browser BLE transport commands bind
to a connection generation, including same-tab reconnects; a retired browser
session cannot be revived by a delayed heartbeat. An unconfigured Handy cannot
connect through a stale UI/API request. Setup offers the shared manager when a
Handy connection needs adding.

Known zero strength is valid silent readiness. Unknown strength, pending
handshakes and unapplied ceilings block startup. Lost strength knowledge during
playback silences the owner and reports unhealthy state. Calibration cannot
overlap another device's playback. Fault Stop zeroes Coyote strength; ordinary
Pause retains it. Both participants' Stop results are joined.

## Deliberately unchanged

The shared motion engine, generated curves, point sampling, Handy timing,
Coyote renderer/profile mapping, ceilings, fades, manual timing offset, and
LLM prompts/providers remain in place. There is no extra motion loop, new core
dependency, automatic reconnect, automatic strength increase, or native
stimulation-pattern vocabulary in this change.

## Evidence

Focused automated cases cover migration and explicit empty persistence;
add/include/exclude/disconnect/remove; controller permissions; default policy
and settings persistence; failed append/Play on either participant; startup
rollback under either policy; continuing after either participant fails;
joined Stop errors; reconnect fencing; and end-to-end engine recovery after
simulated Coyote loss. UI cases cover the icon-only title-row +, backend-confirmed
updates, removing the last connection, and retained excluded panels.

Live API smoke checks on the isolated `-simulate-motion` review build verified
add without selection, simulated Coyote connection, inclusion/exclusion,
silent retained connection, removal, and empty-inventory chat. No motion was
started in this live smoke run. A text-only chat completed through the app with
one Ollama provider call, non-empty text, no repair, no semantic fallback, and
no recorded chat failures. Its first request included a cold model load; it is
not a warm latency benchmark. The standalone review-LLM probe also completed a
real generation.

Final-source validation passed:

- `go test -p 2 ./...` and `go test -race -p 2 ./...`, including architecture,
  motion/transport teardown and leak checks. Final HTTP suite times were
  91.738 s normally and 181.257 s with the race detector.
- `gofmt`, `go vet ./...`, `golangci-lint run` (zero issues), and
  `git diff --check`.
- Frontend typecheck, all **888 tests in 115 files**, localization audit
  (**3,174 keys, five locales**), and the canonical embedded production build.
  Existing React test `act` warnings and the Vite chunk-size advisory remain.
- `CGO_ENABLED=0` core and native-worker builds. Native radio dependencies
  remain in the worker; the core still has 14 modules.

The final running app at `http://127.0.0.1:50233/#/settings/device` was checked
at its normal desktop width and 390 px. The title-row +, protocol alignment,
separators, boxed Coyote notice and reachable Emergency Stop were inspected.
Browser actions exercised add/remove (including the last row), playback
selection, live Settings synchronization, and failure-policy save/reload.
No browser console errors were recorded. Both saved devices are left
disconnected, with Handy selected, Coyote excluded and `stop_all` restored.

The final build passed `scripts/check-review-llm.ps1` with a real Ollama
Gemma 12B generation. With no devices configured, a second text-only request
through the actual browser chat returned non-empty text in **304 ms**, with
first token **126 ms**, one provider call, no repair/fallback, no motion, and
an empty failure list. This is one warmed-model smoke check, not a comparative
performance benchmark. The prompt and motion renderer were not changed.

Evidence is retained under ignored
`.scratch/codex-coyote-fixes/device-state-*` and `.scratch/codex-coyote-state/`,
including final gate logs, chat diagnostics, a redacted trace export, snapshots,
desktop/mobile screenshots and measured asset sizes. The initially
CPU-saturated run is retained as well. After the user authorized stopping the
busy Discord process, all final gates passed at their original timeout values.
No test assertion or repository timeout was weakened. Resource measurements
are recorded in [the scorecard](goal-scorecard.md).

## Hardware status

No real hardware was scanned, paired, connected or commanded. The connection
smoke test used the in-process Coyote simulator. Physical Stop-to-zero latency,
radio-loss decay, BF ceiling enforcement, pairing and dual-device timing still
require a separately authorized hardware session.
