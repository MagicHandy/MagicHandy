# Testing — MagicHandy

## Primary rule

**Every test in this repo uses a real Handy device and analyzes synchronization.**

Full policy: [Domain Rule 04](../domain_rules/04-real-device-sync-testing.md) and [ADR-0014](../adrs/0014-testing-strategy.md).

## Quick checklist (real-device session)

1. Connect device (cloud key or Bluetooth — match active dispatch owner)
2. Run the scenario under test (chat, auto, freestyle, procedural chain)
3. Capture `GET /api/traces`, `GET /api/state`, visualizer observation
4. Verify: dispatch ↔ motion, timeline coherence, buffer health, handoffs
5. Record result in PR or TRACKER session log

## Structural commands (precondition only)

Device E2E sync (Rule 04):

```powershell
$env:MAGICHANDY_DATA_DIR = "c:\dev\git\MyProjects\Handy\MagicHandy\.local-data"  # if key not in AppData
go test -tags=integration ./internal/httpapi -run TestProceduralSyncUninterruptedOnRealDevice -v -timeout 5m
```

These must pass **and** real-device sync analysis must pass before marking tasks done.

## Device test patterns

- `internal/httpapi/e2e_chat_chaos_device_test.go` — reference for device-gated Go tests
- `cmd/retarget-validate` — retarget trace validation on hardware
- `docs/perf-baseline.md` — record RSS **and** sync notes when measuring performance

## Prohibited

- Marking motion/transport/chat tasks complete with only `go test` / Vitest
- Ignoring `starvation_risk` or playhead drift in traces
- Testing on fake transport and claiming feature acceptance
