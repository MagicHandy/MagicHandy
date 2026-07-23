# ADR-0014: Testing Strategy

## Status

Accepted (amended 2026-07-13 — real-device sync policy)

## Context

MagicHandy replaces a Python codebase without its test corpus. Motion and transport quality depends on real Handy firmware buffering, cloud round-trips, and timeline scheduling — behaviors that fake transport cannot reproduce. StrokeGPT-ReVibed regressions were often synchronization bugs invisible to unit tests.

## Decision

### Primary policy — real device + synchronization focus

**Every test performed in this repository must use a real Handy device** and include **synchronization analysis**. See [Domain Rule 04](../domain_rules/04-real-device-sync-testing.md) for the mandatory checklist and completion criteria.

| Principle | Meaning |
|-----------|---------|
| Real device mandatory | Cloud REST or browser Bluetooth (active dispatch owner) for all validation |
| Sync-first analysis | Traces, playhead, buffer health, clock alignment — not just "it moved" |
| CI is structural only | `go test` / Vitest on fakes catch regressions in logic; they **do not** satisfy acceptance |
| No done without device | TRACKER tasks touching motion/transport/chat dispatch stay `[ ]` until device sync test recorded |

### Test layers

| Layer | Location | Command | Role |
|-------|----------|---------|------|
| Structural unit | `internal/**/*_test.go` | `go test ./...` | Parsers, math, invariants — **precondition**, not acceptance |
| Architecture | `internal/architecture/` | `go test ./internal/architecture/...` | Import boundaries, file size |
| Race | All packages | `go test -race ./...` | Concurrency regressions |
| Invariant | `internal/transport/` | Part of `go test` | HSP shape rules |
| HTTP integration | `internal/httpapi/*_test.go` | `go test ./internal/httpapi/...` | Handler wiring with fake transport — **precondition** |
| Goleak | `internal/transport`, `internal/motion` | goleak tests | Goroutine leaks |
| **Real-device validation** | Manual + `e2e_*_device_test.go` | Device connected | **Required acceptance gate** |
| Retarget validation | `cmd/retarget-validate` | Manual CLI | Trace comparison on real device |
| Frontend structural | `frontend/src/**/*.test.ts(x)` | `npm run test` | UI logic — **precondition** for UI-only changes |

### Synchronization analysis (required)

During every real-device test session, reviewers must inspect:

- `GET /api/traces` — dispatch timing, buffer-ahead, starvation
- `GET /api/state` — playhead, motion status, chat auto stamina
- Motion visualizer vs physical device alignment
- Segment handoffs (procedural chain, Chat Auto append/bridge)
- Wall-clock vs HSP timeline divergence (Chat Auto)

### CI pipeline (`.github/workflows/test.yml`)

CI runs structural checks only:

1. `gofmt`, `go vet`, `golangci-lint`
2. `go test ./...`, `go test -race ./...`
3. `CGO_ENABLED=0 go build`
4. Frontend build + Vitest

**CI green does not replace real-device sync testing** for motion/transport/chat changes.

### Conventions

- Table-driven tests for parsers and motion math (structural).
- Fake transport in CI — never require cloud credentials in automated jobs.
- Device tests: use real connection; skip in CI when no device env (local dev must run them).
- PRs touching `motion`, `transport`, `manualqueue`, `httpapi` chat/modes: author documents real-device sync session in PR description.
- New timing behavior: add trace rows testable on device export.

### Coverage

No percentage gate. Critical paths need structural tests **and** real-device sync evidence before behavior changes ship.

## Consequences

### Positive

- Sync bugs caught before user reports.
- Traces become standard evidence, not optional debugging.
- Aligns team on what "tested" means.

### Negative

- Every contributor needs device access for motion work.
- Slower validation cycle than fake-only testing.
- CI cannot fully gate motion quality.

### Neutral

- Performance baselines in `docs/perf-baseline.md` remain manual; include device sync notes when recording.

## References

- [Rule 04 — Real device sync testing](../domain_rules/04-real-device-sync-testing.md)
- [`procedural-chat-motion-analysis.md`](../procedural-chat-motion-analysis.md)
- [`hsp-v4-invariants.md`](../hsp-v4-invariants.md)
