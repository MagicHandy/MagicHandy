# Rule 02 — Procedural vs Library Motion Routing

## Rule

Chat motion dispatch follows a **single router** in `httpapi.dispatchChatMotionForResult`:

```
if shouldUseChaoticChatMotion(command, settings) {
    → dispatchChatChaoticMotionAsync  (manualqueue.Player + HSP)
} else {
    → dispatchChatMotion              (motion.Engine + retargeting)
}
```

### When procedural (chaotic) path is used

All conditions must be true:

1. `settings.Motion.MotionGenerationMode == "procedural"`
2. `motion` is non-nil
3. `motion.action` is `start`, `target`, or `stop`

### When library/engine path is used

- `motion_generation_mode == "library"`, **or**
- `motion.action` is `none` or empty, **or**
- Procedural mode but action does not match start/target/stop

Library path uses `motion.Engine` for semantic retargeting and pattern playback. Procedural path **bypasses** the engine and encodes `[]TimedPoint` through `manualqueue.Player`.

### ModeChat keepalive

`POST /api/modes/start` with `mode: chat` restarts the **last chat engine target** after transport recovery. It does **not** drive procedural HSP sessions. Procedural chat has no equivalent keepalive today (known gap P0-B).

### Dispatch throttle (procedural only)

`chatChaosDispatchMinInterval` = **450 ms**. Dispatches inside this window may be skipped; callers must not report `Applied: true` unless motion reached the player (fix tracked in feature 001).

### Transport wrapper

Procedural players use `newMotionCommandTransport()` which wraps the selected transport with motion-command recording for traces.

## Rationale

Two stacks exist for historical reasons: engine path for library/retarget semantics, procedural path for LLM-driven physics waypoints. The router must stay explicit so tests and diagnostics know which playback stack is active. Source: `internal/httpapi/chat.go`, `chat_chaos.go`.

## Examples

| Settings mode | Motion action | Path |
|---------------|---------------|------|
| procedural | `start` + physics fields | `manualqueue.Player` |
| procedural | `stop` | `manualqueue.Player` (stop session) |
| library | `start` + `padrao_id` | `motion.Engine` |
| procedural | `none` | Engine no-op path |
| library | `start` + `velocidade` | **Rejected** at parse |

## Edge cases

- `operation_mode: auto` (Chat Auto) always uses procedural transport via `chat_auto.go`, not `dispatchChatMotionForResult` for segment playback.
- `operation_mode: hybrid` uses `dispatchChatMotionForResult` per chat message.
- Switching `motion_generation_mode` in settings does not migrate an active player; user should stop motion first.
- `shouldUseChaoticChatMotion` returns false for `action: none` even in procedural mode — engine handles library-style no-ops.

## References

- `internal/httpapi/chat.go` — `dispatchChatMotionForResult`
- `internal/httpapi/chat_chaos.go` — `shouldUseChaoticChatMotion`
- ADR-0011 (layered architecture, dual-stack exception)
- [`procedural-chat-motion-analysis.md`](../procedural-chat-motion-analysis.md) §1–2
