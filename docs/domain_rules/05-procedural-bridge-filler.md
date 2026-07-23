# Rule 05 — Procedural Bridge Filler

## Rule

When procedural chat motion (`chatChaos.player`) is running and the timeline has **less than 10 seconds** remaining, with **no dispatch in flight**, MagicHandy must append a **bridge filler** segment (~30s) via `AppendExtension` — not restart the player.

## Rationale

Hybrid procedural chat segments last 2.5–7s. Without a bridge, motion stops between user messages while the LLM responds. Chat Auto already solves this with `chatAutoMaybeLoopBridge`; procedural chat must match that continuity guarantee.

## Filler physics

| Field | Value |
|-------|-------|
| `tipo_batida` | `fluido` |
| `regiao` | Same as last successful dispatch |
| `velocidade` | `max(20, last_velocidade - 15)` |
| `stroke_range` | Preserved from last dispatch |
| Duration | ~30 seconds |

## Stop conditions

Bridge filler stops when:

1. User sends **stop** or new dispatch (generation bump)
2. Transport drop / player stop
3. Explicit mode stop (`cancelChatChaosMotion`)

## Debounce

At most one bridge per **5 seconds** (`lastBridgeAt`), matching Chat Auto.

## Trace

Successful bridge append logs trace row `chat_chaos_bridge_filler` via `motion.MotionDebugLog`.

## Examples

- Segment ends in 8s, user silent → bridge appends 30s fluido filler in same zone.
- User sends new message during filler → new dispatch replaces via generation token; bridge watcher stops.

## Edge cases

- **Short segments (<10s total):** Bridge may fire soon after start because remaining is already below lead threshold; debounce prevents spam.
- **Append failure:** Falls through to player restart path on next user dispatch only; bridge logs `bridge_append_failed`.
- **No prior physics:** Bridge skipped until first successful `playChatChaoticMotion`.

## Related

- Feature 002 phase 0
- [`procedural-chat-motion-analysis.md`](../procedural-chat-motion-analysis.md) §18 P0-B
- Rule 04 (device sync acceptance)
