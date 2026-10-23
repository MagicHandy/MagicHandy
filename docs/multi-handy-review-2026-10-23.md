# Multi-Handy implementation review — 2026-10-23

## Scope and design review

The candidate supports the existing primary Handy plus seven named additional
connections. Cloud, native Bluetooth and Intiface can be mixed; the primary can
also use Browser Bluetooth. All sources still enter one shared engine and one
sampler. Additional Intiface outputs share the hub instead of opening competing
clients. Saved inventory, connected state and playback participation remain
separate. See [ADR 0054](decisions/0054-named-handy-connections.md).

Claude Opus 5.5 Max implemented the core frontend changes through a file-only CLI
session. It reached its session limit before finishing localization/tests.
Codex completed Japanese/Chinese catalogs and tests, reviewed the integration,
and corrected host permission affordances and stale Intiface selections. All
five locales pass the catalog audit.

The backend review corrected several integration gaps before submission:

- Read each Cloud slider's own startup geometry; never broadcast a first
  slider's lead-in to another slider or an electrical/vibration output.
- Dispatch independent appends and companion starts concurrently; include the
  slowest participant in refill latency and honor every sampling constraint.
- Bind engines to connection generations, cancel pending native connections on
  Stop, and keep removed Intiface owners from retaining a pacer or claim.
- Stop prepared/excluded devices and drain every available Stop even when a
  different configured transport cannot be constructed.
- Preserve named inventory during old full-settings writes, clone mutable
  slices, redact all instance credentials, and list additional Intiface leases.

## Automated validation

Local Windows checks: `go test ./...`, `go vet` on affected packages,
`golangci-lint run ./...` (zero issues), architecture/import-boundary tests,
`CGO_ENABLED=0` build, TypeScript, localization and the full frontend suite
(122 files / 966 tests). Linux race and release gates run in CI before merge.

New regressions exercise independent persistence/removal, duplicate private
identities, the inventory cap, participating-model calibration, old lease
rejection after repeated reconnects, excluded-device Stop, partial Stop
construction failure, private instance-bound discovery, three-device failure
policies, concurrent append cancellation, late Intiface binding/claim release,
startup from different positions and failed companion preparation. A Cloud
HTTP fixture verifies that elapsed timeline alignment includes clock-refresh
time; latency tests retain the slowest companion's budget.

Frontend tests cover returned-ID navigation, adding without connecting,
independent names/keys, supported protocol choices, host permissions, named
Intiface disconnection and backend-owned playing/retired state. Existing
single-device, Coyote, vibrator and Stop tests remain in the suite.

## Review app and motion evidence

An isolated `-simulate-motion` build on port 50269 used a native Handy RPC
simulator, a Cloud fake and the in-process Intiface simulator together. The
primary placeholder was removed to also exercise an all-named inventory.
No real hardware was connected and no user credentials were copied.

The final app test completed 28 operations, three two-second runs at 30%
(full sweeps, base-anchored variety, pace wave), Stop after each, and three
disconnect/reconnect cycles for Cloud and Intiface. Reconnects remained stopped.
The observed API median was 13.6 ms, maximum 266.3 ms; Stop responses were
25.8 / 15.8 / 14.9 / 0.8 ms. These are local simulation timings, not network or
physical Stop measurements. Native discovery/connect was also exercised in
the visible UI against its in-memory simulator.

Ignored artifacts retained under `.scratch/multi-handy/`:

- `app-validation.json`, `app-traces.json`: operations, snapshots and the
  exported shared-engine command trace.
- `startup-capture.json`, `startup-review.png`: synthetic Cloud positions
  90% and 45% each approach 20% on their own bounded paths, then receive
  identical main samples. The non-telemetry peer receives no foreign lead-in.
- `mixed-motion-review.png`: all three app patterns rendered from actual
  transport-command points, including queued samples canceled by Stop.
- `devices-desktop.jpg`, `devices-mobile.jpg`, `settings-mobile.jpg`: 1523px
  desktop and 390×844 settings/panel checks. Names, protocol/status and
  participation stay distinct; the panel leaves the permanent Stop reachable.

The overview and individual three pattern traces were inspected. Startup
anchors differ as expected; main frames agree. The sampled sweep/pace curves
and changing base-anchored amplitudes retain their shapes. No generator recipe
or LLM language rule changed. This is commanded-path review, not proof of
physical smoothness or simultaneous arrival.

`check-review-llm.ps1` completed real generation through the configured local
Ollama Gemma 12B. A text-only app chat returned “I am ready to continue our
conversation.” in 296 ms (128 ms first token), without repair/fallback or motion.

## Weight and remaining limits

Matched Windows amd64 `CGO_ENABLED=0 go build` binaries: 32,303,104 B before,
32,555,520 B candidate (+252,416 B, 0.78%). Complete canonical UI:
2,731,986 → 2,760,194 B (+28,208 B, 1.03%); main JavaScript gzip:
338,685 → 341,875 B (+3,190 B, 0.94%). No dependency was added. A post-test
three-connection review process sampled 90.1 MB working set / 66.4 MB private
memory; this is not a matched idle/soak comparison. No RSS/cold-start improvement
is claimed. Unconfigured instances do not start transport workers.

Two-or-more physical Handy validation is still pending, including mixed network
latency, radio capacity and long-running synchronization. Shared calibration
deliberately limits every participant to the slowest normalized model profile.
Only one Browser Bluetooth Handy is supported. The known Intiface 3.2.2
Stop limitation remains; this change does not claim to repair upstream device
Stop behavior. Loaded Coyote/vibrator and worker-crash limitations are unchanged.
