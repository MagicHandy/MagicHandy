# Architecture review implementation

This branch implements the November 17 review against release commit `bafc7ec4`. Its two highest-priority regressions both reproduced on that commit. Resuming a paused run could restore a 0–100% stroke window, and saving an older Diagnostics draft could undo a newer live adjustment. The permanent tests exercise the HTTP entry points and inspect the commands the simulated owner actually received. [Resume regression](../internal/httpapi/motion_limits_resume_test.go) · [Settings regressions](../internal/httpapi/settings_patch_regression_test.go)

## Changes and evidence

| Review item | Behavior and verification |
| ---: | --- |
| 1. Paused limits | An edit while paused sends zero commands. Resume reads saved settings under the settings lock and checks the Stop generation before dispatch. Its first command sets 30–65% with reversal in the regression, and the target respects the new speed cap of 40. |
| 2. Concurrent settings edits | The settings form sends changed fields and their original values in `PATCH /api/settings`. One conflict rejects the entire patch with 409. Unrelated saves preserve live controls and stored credentials. Replacing a write-only voice key also requires the settings revision seen by the editor. The older full-replacement PUT contract remains available to existing clients. |
| 3. Unplayable frames | A timing-fit failure after Play retires the run and sends one recovery Stop. The regression retains the owner’s 50 ms command-spacing error and verifies that the last command is a fault Stop. |
| 4. Physical Stop outcome | Loopback and protected HTTP, console, and the global button distinguish confirmed, pending, unconfirmed, and no device. Confirmation requires a successful Stop result from the addressed owners. A retired session returning success for an ignored command fails that check. Protected responses expose only the public outcome. |
| 5. Simulator isolation | Startup propagates simulator initialization errors and closes previously started listeners. Simulated Intiface and Autoblow connections require their local endpoints. Failure-injection tests intercept the connection path and count outbound attempts. |
| 6. Interactive handoff | Edits at the same uncommitted cursor replace each other. Dispatch respects each owner’s lead and frame size, and fixed media retains its deeper buffer. In the fixed-clock ten-edit test, the release selected 10,000 ms for edit ten. The first differing submitted point is now at 2,000 ms, inside the 2,500 ms fixture bound. Already accepted commands remain identical. |
| 7. Creative v2 preparation | The compiler reuses per-leg curve data and fixed-size polynomial root buffers. The release and candidate produced identical points, spacing and kinematic metrics across the 135-case atlas. |
| 8. Playback clock | Local elapsed-time arithmetic preserves Go’s monotonic clock reading. Diagnostics and wire timestamps convert at their boundaries. A test moves the device wall clock forward 12 hours and back 24 hours while elapsed playback advances steadily. |
| 9. Motion observations | Browser observations omit resolved program and pattern data before cloning. SSE subscribers share a projection for at most one 125 ms tick. A fresh capture follows a changed Stop sequence. Redaction and remote-command delivery still run separately for each requesting client. |
| 10. Device lifecycle | `internal/deviceapp` coordinates engine admission, retirement, settings refresh, Resume and Stop. The HTTP layer retains authorization, decoding and presentation. Lifecycle and integration tests exercise a blocked Resume, cancellation, owner replacement and shutdown. |
| 11. Device panel ordering | Backend device snapshots carry an epoch and revision. Intiface, Coyote and Autoblow panels allow one outstanding poll per subscription, abort on cleanup and reject overtaken replies. Tests cover reordered action and poll replies, a new epoch, and a panel closing mid-request. |
| 12. Idle reads | Playback readiness inspects selected owners without creating sessions. A never-connected named Handy stays unallocated. Library, media, memory and conversation aggregates share completed reads until a datastore write advances the revision. Live scan state and client-specific responses are assembled on each request. A waiting client can cancel independently, and a commit during a read prevents caching the obsolete result. |
| 13. Trace ownership | Trace rows own nested anchors and pace limiters. Mutating input, the returned row or an export preserves the ring’s copy. Dispatch uses a store-only path when it discards the returned row. |
| 14. Browser delivery | Secondary routes load on demand. Correction 2026-11-25: alpha.89 also moved seven shared stylesheets into route chunks. Chat's Autopilot card, the persona switcher and the device connection choices then rendered unstyled until their owning route had loaded. Alpha.90 loads those sheets with the app shell again, and a check now enforces stylesheet ownership. Only the Modes and Videos sheets still load with their routes. The global Stop stays mounted while routes load or fail. Hashed JS and CSS support gzip, representation-specific ETags, immutable caching, conditional requests and ranges. The local and remote HTML shells use `no-store`. |
| 15. Idle readouts | Now Playing and Diagnostics use the same interpretation of backend readiness. A ready owner with no engine reads idle. A failed readiness check reads unavailable. Both cases have paired display tests. |
| 16. Import rules | Depguard matches both direct and nested files. A negative fixture runs the pinned linter on Ubuntu and Windows. A separate Go syntax-tree check rejects concrete transport constructors in production motion code, including references used as values. |
| 17. Tagged Labs CI | CI builds all three tagged review commands, lints tagged sources, runs the inert 350 ms speed scenario and verifies that it wrote a report. The manual workflow runs and retains the larger transport comparison. |
| 18. Frontend and size checks | React hook ordering is linted across application code. Long-lived state, device polling and Stop hooks also check effect dependencies. CI publishes the existing Go size advisories without changing the thresholds. The new npm dependencies are development tooling. |
| 19. Long-running simulation | Runtime fake owners keep the latest 64 commands and results with separate lifetime counters. Unit fixtures can retain an entire command history. The paired one-hour runs record core memory, lead at the accepted tail, Stop acknowledgements and subsequent command counts. |

## Visual motion review

The shared compiler exported 135 cases for both revisions. The complete exports had the same SHA-256, `ca9b4a0156608bd127f5017416ff293766ea01d02e056648f7b6a4af6eac6f72`. Rendering produced 129 distinct plots and nine overview sheets. Inspection included every overview and the following detail views. [Atlas procedure](motion-visual-review.md)

- **Roaming narrow strokes, Original at 10%.** The encoded points follow the narrow stroke as its center moves. Peak velocity was 113.4% travel/s and peak acceleration was 2,587% travel/s². Reported velocity and acceleration jumps were zero.
- **Maximum combined character, Handy 2 Pro at 85%.** The encoded path follows the reversal peaks. Peak velocity was 358.5% travel/s, peak acceleration was 7,692% travel/s² and peak jerk was 518,235% travel/s³. Reported velocity and acceleration jumps were zero.

The transport comparison uses simulated Cloud, Intiface and a 100 ms Intiface timing floor. The runtime speed scenario uses an injected 350 ms append delay. The atlas, timing checks and exported traces measure commanded motion. Physical travel, device firmware latency and perceived feel still require a hardware session.

## Verification scope

The Go checks cover formatting, vet, default and tagged golangci-lint, the default suite, the race detector and a CGO-disabled core build. The release-line import fixture runs on Linux and Windows. The frontend checks cover TypeScript, localization, both Hooks rules, 1,136 tests and a clean production build. Native PowerShell 5.1 and 7 exercise the installer scripts. The Ubuntu voice matrix runs all seven tests on both Python 3.10 and 3.11.

The Go source vulnerability scan found zero reachable advisories. The production npm audit reported zero advisories. A full npm audit reported the same 11 development-tool findings on the release base and this branch, including two critical findings on the Vitest and Tinypool test stack. Upgrading that existing test stack remains follow-up work.

The packaging workflow also builds a version-stamped unsigned installer and portable ZIP, scans each packaged Go executable, and exercises current-user install, upgrade, keep-data uninstall, default machine install and purge/reinstall in a clean Windows environment. See the [Windows packaging workflow](../.github/workflows/package-windows.yml).

## Reproduction

Use Go 1.26.11 and golangci-lint 2.12.2. These benchmarks operate on in-process transports and a synthetic pattern library.

```sh
go test ./internal/motion -run '^TestBurstRetargetsUseFirstUncommittedHandoff$' -v
go test ./internal/motion -run '^$' -bench '^BenchmarkCreativeV2Timing$/^speed-75$/^optimized-true$' -cpu=1 -count=3 -benchmem
go test ./internal/motion -run '^$' -bench '^BenchmarkProgramObservation$' -cpu=1 -count=3 -benchmem
go test ./internal/httpapi -run '^$' -bench '^BenchmarkPopulatedStateObservers$' -cpu=1 -count=3 -benchmem
python scripts/check-depguard.py
mkdir -p .scratch
MAGICHANDY_SPEED_RUNTIME_REPORT="$PWD/.scratch/speed.json" go test -tags magichandy_labs ./internal/motion -run '^TestSpeedReviewRuntime$' -count=1
MAGICHANDY_TRANSPORT_PARITY_REPORT="$PWD/.scratch/parity.json" go test -tags magichandy_labs ./internal/motion -run '^TestTransportParityReview$' -count=1
```

Release and candidate measurements, sample windows and remaining budget misses are recorded in the [goal scorecard](goal-scorecard.md#2026-11-17--architecture-review).
