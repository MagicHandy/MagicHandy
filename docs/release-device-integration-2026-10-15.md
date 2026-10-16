# Device UI release integration — 2026-10-15

The final release must include Claude's current device UI work, in addition to
the native Bluetooth and motion/model changes. `codex/release-device-ui`
combines these changes without modifying Claude's working directory:

- `654c37a5` and its seven commits after `5938a0cb`: device-set playback,
  companion timing helpers, shared Intiface leases/claims, vibrator output and
  its settings.
- `505f0d5c`: the device UI snapshot from `claude/device-menu-redesign`, including
  the previously uncommitted device kit and ADR 0048. The snapshot recorded 103
  changed/deleted/untracked paths and verified their source hashes. The original
  worktree remained unchanged at the follow-up check.
- `fa125b09`: integration with motion-review commit `5dc2dc2f`. Only two
  documentation files conflicted; both sets of evidence were retained. Stop
  wording now distinguishes a server request from physical delivery.
- The panel title retains the requested small + shortcut. It opens the existing
  Add device page while another device type is available. It does not connect a
  device or create a second setup form.

## UI and behavior checks

The new Devices panel and Settings > Devices are present in the combined app.
Each device has its own setup page, with one shared descriptor used for its
status, row, glyph and actions. Immediate saves, boxed notices, separators,
compact levels and the phone sheet are preserved. The old connection-manager
components are removed and only one rebuilt canonical UI bundle ships.

The simulator review exercised adding Coyote and a vibrator, choosing the
in-process simulated vibrator, and saving its follow mode from Speed to Depth.
The saved backend value changed to `depth`; the new devices were still excluded
from playback and vibrator intensity/output remained zero. The device panel was
inspected at 1280×720 and 390×844. Emergency Stop remained reachable while the
phone sheet was open and returned Stopped. The title-row + opened Add device
without connecting anything. No real hardware was connected or moved.

Validation: full Go and race suites, gofmt, vet and golangci-lint pass; the
Windows and Linux core builds pass with `CGO_ENABLED=0`. Frontend typecheck,
build and 906 tests in 117 files pass. The eight DevicePanel tests and typecheck
were rerun after adding the shortcut. Existing CI still needs to validate the
published integration head; green checks on the motion-only PR do not certify
this larger combination.

The final review app at `http://127.0.0.1:50247/#/settings/devices` uses fresh
isolated data and the motion simulator. It uses the available local Gemma 12B
model, with LLM motion and voice disabled. `check-review-llm.ps1` completes a
real generation. A separate text-only app chat request returns `13` in 234 ms,
with one provider call, no repair/fallback, and no active motion engine. Evidence
and screenshots remain under ignored `.scratch/release-integration/`.

## Upstream Stop review and release acceptance

The Handy Stop defect is reported in
[Buttplug issue 971](https://github.com/buttplugio/buttplug/issues/971), with a
[public review candidate](https://github.com/mapledaemon/buttplug/commit/2698303).
The candidate passes eight specific Stop regressions, 60 server-library tests
and 43 integration tests (one existing ignored test). The three original
reproductions fail against the unmodified implementation. These tests use
recording hardware, not a Bluetooth device.

The upstream contribution policy requires issue discussion and maintainer
approval before a PR. The issue asks for that review. The installed Intiface
is unchanged. The prototype is not bundled
with MagicHandy and does not yet solve deferred-command ordering or aggregate
Stop error propagation. It does not establish physical stopping or improved
motion feel.

Before publication, recheck Claude's source for subsequent changes, finish
review/CI for the combined branch and dependency update, and resolve the
Intiface Stop/timing acceptance findings with an observed unloaded comparison.
The final release is not complete merely because the UI integration is green.
