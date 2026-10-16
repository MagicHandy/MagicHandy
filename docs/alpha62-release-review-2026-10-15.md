# Alpha.62 release review — 2026-10-15

The candidate combines PRs 319–324 on `codex/release-alpha62`: COYOTE and
linked sessions, native Handy Bluetooth, the motion/model review, Claude's
device UI, the network/crypto update and physically accepted Intiface smoothing.
Merge conflicts in the dependency manifests retained the Bluetooth worker's
dependencies and selected the newer reviewed `golang.org/x` versions; `go mod
tidy` regenerated checksums. The original contributor worktrees are preserved.

## Acceptance evidence

- [Native/linked hardware](native-linked-hardware-review-2026-10-15.md): repeated
  normal Handy and COYOTE reconnects, unloaded playback, Stop, and both linked
  failure policies. Crash recovery with a live companion remains limited.
- [Local/hosted models](release-motion-review-2026-10-15.md): all five modes,
  compound edits, hold/release, long history, delayed review and refusal retry.
  Local prompt budgets remain unchanged; no language rules or extra repair
  calls were introduced. Real OpenRouter and separately billed OpenAI accounts
  were not exercised.
- [Intiface smoothing](intiface-smoothness-review-2026-10-15.md): rejected dense
  and adaptive variants retained; preferred spacing preserves fast turns and
  the actual device floor. The operator accepted the final full-sweep and
  Creative v2 production-build runs at 35%. Higher speeds were rendered, not
  physically assessed. This is not proof of a hardware Stop.
- [Device UI](release-device-integration-2026-10-15.md): Claude's 103-path
  snapshot has no subsequent source differences. Desktop/phone panel checks,
  the small + flow, immediate settings, stopped state and simulator output
  passed. Follow-up review fixed the manual test's retired pattern IDs and
  made its visible/submitted speed follow backend limits.
  The full new glyph set was reviewed after the maintainer flagged small,
  off-center symbols. Grid-centered SVG tiles now render at 20/24/28px;
  Coyote uses a lightning bolt and asymmetric drawings were rebalanced.
  Desktop and 390px phone screenshots show readable tiles and reachable Stop,
  with no horizontal overflow. Visible Settings tile/SVG centers match exactly.
  The Intiface notice was saved as hidden and restored through notice settings.
- The current simulator review app at `http://127.0.0.1:50254` started all three
  manual choices from the UI and stopped through the permanent Stop button.
  A text-only app chat returned `13` in 225 ms (first token 128 ms), with LLM
  motion Off. The readiness probe also completed real local Gemma generation.
  This isolated profile contains no copied credentials and connects no hardware.
- Full Go tests and the full race suite pass. Vet and golangci-lint pass with
  zero lint issues. Frontend typecheck, localization, build and 910 tests in
  117 files pass. The source vulnerability scan reports zero called/package
  vulnerabilities; the unused openpgp module advisory remains. Windows and
  Linux CGO-disabled core and worker builds are checked separately from CI.

The intermittent installer timeout was not reproduced locally. The runtime
fixture now selects both ports just before launch, uses plain console output,
and reports process-exit/startup errors. Both PowerShell 5.1 and 7 suites passed
after that change; the alpha.62 policy change is revalidated by release CI.
No hard gate was removed or downgraded. Published CI and exact package lifecycle
results still determine merge/tag readiness.

## Known limits and release policy

[ADR 0049](decisions/0049-alpha62-external-stop-limitation.md) records the
maintainer's direction to leave the Intiface Stop fix upstream and continue
publication work after the smoothness comparison. R23 remains Critical. The
app's local cancellation and Stop dispatch requirements remain mandatory, but
Central 3.2.2 can acknowledge a Handy Stop without a device Stop. This is
disclosed in settings and the public notes; the notice uses the shared backend
catalog and persisted dismissal preferences.

COYOTE testing used disconnected output leads. Loaded waveform/decay, physical
radio loss and quantitative device alignment are not verified. Vibrator output
has simulation coverage only. Recovery from a native worker crash can require
stopping and disconnecting both devices. None of these is presented as complete
physical acceptance.

[ADR 0014](decisions/0014-public-windows-signing-gate.md) extends the reviewed
unsigned alpha policy to alpha.62 only. Main-tip provenance, green quality and
package CI, source/binary vulnerability scans, exact-artifact Defender,
manifests, checksums and the Windows install/upgrade/uninstall lifecycle remain
required. Only the standard setup, portable ZIP and checksum file are public.

## Budget

With Go 1.26.9, Windows amd64, `CGO_ENABLED=0`, `-trimpath -buildvcs=false`,
`-ldflags '-s -w'`: core **23,236,608 B**, Bluetooth worker **3,093,504 B** and
embedded UI **2,679,035 B**. These include the final glyphs, notice and dependency
update. Public releases retain symbol tables for scanning and are larger than
these stripped budget builds. No new RSS or cold-start claim is made.
