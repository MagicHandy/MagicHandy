# Phase 2 — Dispatch Semantics and Throttle Fixes

## Objective

Fix user-visible false positives on motion dispatch and document/improve procedural transport recovery (P0-B, P1-C).

## Prerequisites

- Phase 1 complete
- Read [`procedural-chat-motion-analysis.md`](../../../procedural-chat-motion-analysis.md) §12, §18 (P0-B, P1-C)
- Read [ADR-0013](../../../adrs/0013-error-handling.md)

---

## Task 2.1 — Fix throttle silent Applied:true

**Action:**

1. In `internal/httpapi/chat_chaos.go`, trace `chatChaosDispatchMinInterval` (450 ms) path.
2. When dispatch is skipped due to throttle, return `Applied: false` (or distinct flag) to the UI — not `Applied: true`.
3. Ensure SSE/chat response JSON reflects whether motion actually reached the player.
4. Add test in `chaos_session_test.go` or `chat_test.go` for two rapid `target` commands.

**Acceptance criteria:**

- Second dispatch within 450 ms does not report success unless motion was queued
- Test asserts response `applied` / dispatch metadata

---

## Task 2.2 — Procedural transport recovery gap

**Action:**

1. Document current behavior: `playChatChaoticMotion` → `NotifyChatStop()` vs library keepalive (`modes` chat keepalive).
2. Propose minimal recovery: either hook procedural player into transport recovery callback OR document as known limitation in `procedural-chat-motion-analysis.md` §18 with user-facing note in Settings/Diagnostics.
3. If implementing recovery, add trace row on reconnect; do not auto-restart after explicit user stop.

**Acceptance criteria:**

- ADR or analysis doc updated with chosen behavior
- If implemented: test with fake transport simulating disconnect/reconnect
- If deferred: `[-]` task with link to follow-up feature in TRACKER

---

## Task 2.3 — Consecutive target dispatch tests

**Action:**

1. Add integration test: hybrid chat, procedural mode, two `target` LLM commands 200 ms apart.
2. Assert player receives expected number of HSP sessions or append operations.
3. Run `go test ./internal/httpapi/... -run Chaos -v`.

**Acceptance criteria:**

- Test passes in CI with fake transport
- Documents expected chaining behavior in test name/comments

---

## Phase completion

- Update feature TRACKER; central TRACKER progress toward 6/8
