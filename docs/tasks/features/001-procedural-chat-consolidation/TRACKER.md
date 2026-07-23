# Feature 001 — Procedural Chat Consolidation — Task Tracker

> See [central TRACKER](../../TRACKER.md).

## Dependencies

None. Informed by [`procedural-chat-motion-analysis.md`](../../../procedural-chat-motion-analysis.md) §18.

## Feature Status

| Phase/Task | Description | Progress | Status |
|------------|-------------|----------|--------|
| Phase 1 | Legacy cleanup and documentation alignment | 3/3 | Done |
| Phase 2 | Dispatch semantics and throttle fixes | 0/3 | Pending |
| Phase 3 | Chat Auto stamina/clock unification | 0/2 | Pending |

## Task Files

- [fase-1-legacy-cleanup.md](./fase-1-legacy-cleanup.md)
- [fase-2-dispatch-semantics.md](./fase-2-dispatch-semantics.md)
- [fase-3-chat-auto-state.md](./fase-3-chat-auto-state.md)

## Task Summary

- [x] Task 1.1 — Legacy `GenerateChaoticWaypoints` moved to test-only (`chaos_waypoints_test.go`)
- [x] Task 1.2 — Dual-stack documented in `contract.go` + README Motion paths
- [x] Task 1.3 — `proceduralMotionTarget` maps velocidade/regiao/stroke_range → `MotionTarget.AreaFocus`
- [ ] Task 2.1 — Fix throttle 450 ms silent `Applied: true` behavior
- [ ] Task 2.2 — Bridge filler → moved to [feature 002 phase 0](../002-semantic-zoning-director-actor/fase-0-bridge-filler-procedural.md)
- [ ] Task 2.3 — Add tests for consecutive `target` dispatches under throttle
- [ ] Task 3.1 — Unify stamina sources (prepared vs live tick)
- [ ] Task 3.2 — Align segment wall-clock vs HSP timeline for prefetch/bridge

## Notes

- Device sync validation: `TestProceduralSyncUninterruptedOnRealDevice` PASS (2026-07-13)
- Task 2.2 superseded by feature 002 phase 0 (bridge filler + semantic pipeline)
