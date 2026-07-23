# Rule 04 — Real Device Testing with Synchronization Focus

## Rule

**Every test performed in this repository must use a real Handy device** (cloud REST or browser Bluetooth — whichever is the active dispatch owner in settings). Synthetic-only validation (fake transport, mocks, CI green alone) **does not count** as a completed test.

Each test session must include **explicit synchronization analysis** before the result is accepted.

### What counts as a test

| Activity | Real device required? |
|----------|----------------------|
| Feature task acceptance | **Yes** |
| Bugfix verification | **Yes** |
| Manual QA / exploratory session | **Yes** |
| Regression check after motion/transport/chat change | **Yes** |
| `go test ./...` in CI or locally | Runs on fake/mocks for structural regression only — **not sufficient alone** |
| Frontend Vitest unit tests | **Not sufficient alone** for motion/sync features |

### Minimum sync analysis per session

Record observations (in PR notes, TRACKER session log, or `docs/perf-baseline.md` when relevant):

1. **Command ↔ motion alignment** — LLM/chat dispatch (`Applied`, action) matches physical device response within expected latency.
2. **Timeline coherence** — playhead / visualizer position tracks device movement without visible lead/lag drift during sustained playback.
3. **Buffer health** — no `starvation_risk` or audible/visible stalls; check `GET /api/traces` for buffer-ahead and dispatch rows.
4. **Clock sources** — when testing Chat Auto or procedural chains, note whether wall-clock (`segmentEndsAt`) and HSP timeline (`TimelineEndMS - playheadMS`) stay aligned; flag early/late bridge or prefetch.
5. **Segment handoff** — consecutive `target` or append operations chain without gap, double-stop, or phase jump (retarget invariants).
6. **Stop/pause semantics** — user stop, chat `stop`, and transport recovery behave as documented; procedural path recovery gap (P0-B) documented if observed.

### Required tooling during real-device tests

```powershell
# Traces (primary sync evidence)
Invoke-WebRequest http://127.0.0.1:49717/api/traces

# Live state
Invoke-WebRequest http://127.0.0.1:49717/api/state

# Transport diagnostics
Invoke-WebRequest http://127.0.0.1:49717/api/transport/diagnostics
```

Use the motion visualizer in the UI and, when retargeting changes, `cmd/retarget-validate` with exported traces.

### Test completion criteria

A task is **not** `[x]` Done in the TRACKER until:

- Real device session executed
- Sync analysis checklist addressed (pass or documented known issue with ticket)
- CI still green (structural regression)

## Rationale

Handy cloud latency, HSP batching, firmware buffering, and the procedural dual-stack cannot be validated with fake transport alone. StrokeGPT-ReVibed regressions repeatedly came from timing and sync assumptions that unit tests missed. Synchronization is the primary quality dimension for this product.

## Examples

**Valid completion — hybrid procedural chat:**

1. Send chat message → `target` with new `regiao`
2. Observe device reaches new zone within lead window (~300 ms generation + transport)
3. Export traces; confirm single HSP stream or documented chain stop/start
4. Note playhead continuity on visualizer

**Invalid completion:**

- "All `go test` pass" with no device session
- Device moved but traces not reviewed for buffer starvation
- Chat UI shows `Applied: true` but device unchanged (throttle bug — must be flagged)

**Chat Auto sync session:**

- Run auto mode ≥2 minutes
- Compare stamina UI vs perceived segment length
- Log if bridge fired early/late relative to timeline end

## Edge cases

- **No device available:** stop and report blocker; do not mark motion/transport tasks done. Structural `go test` may still run.
- **Bluetooth vs cloud:** test on the dispatch owner configured for the session; switching owners mid-test invalidates comparison.
- **Intiface path:** same sync rules apply when Intiface is the motion transport.
- **CI PR gate:** merge requires CI green **plus** author attestation of real-device sync test for motion/transport/chat-touching PRs.

## References

- ADR-0014 (testing strategy)
- [`procedural-chat-motion-analysis.md`](../procedural-chat-motion-analysis.md) §12–13, §18 (P1-D, P1-H)
- [`hsp-v4-invariants.md`](../hsp-v4-invariants.md)
- Rule 02 (procedural vs library routing)
- Rule 03 (Chat Auto clocks)
- `cmd/retarget-validate`
- `internal/httpapi/e2e_chat_chaos_device_test.go` (device test pattern)
