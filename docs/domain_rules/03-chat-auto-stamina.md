# Rule 03 — Chat Auto Stamina Semantics

## Rule

Chat Auto (`operation_mode: auto`) drives **segment duration** from a stamina model (0–100). Stamina is a **session state variable**, not a transport command. Intensity scale here is **1–10**, not the hybrid chat 1–100 physics scale.

### Drain and recovery rates

| Constant | Value | Meaning |
|----------|-------|---------|
| `StaminaMinDrainPerSec` | 0.5 | At intensity 1: 1 point / 2 s |
| `StaminaMaxDrainPerSec` | 2.0 | At intensity 10: 1 point / 500 ms |

`DrainRatePerSecond(intensity)` linearly interpolates between min and max for intensity 1–10.

### Net rate

`StaminaNetRatePerSecond(intent, stamina)`:

- **Positive** → draining
- **Negative** → recovering (only when `stamina < 100` and `IsRecoveringIntent(intent)`)
- At **stamina 100**, recovery is impossible — always drain so blocks have finite duration

### Recovering intent

`IsRecoveringIntent` is true when:

- `intensidade` 1–3, or
- `velocidade` 1–3, or
- `humor == desejando` with `intensidade <= 4` and `velocidade <= 4` (or zero)

### Stamina commit paths

| Function | When | Behavior |
|----------|------|----------|
| `ApplyStaminaCommit` | Normal LLM segment | Pose resolve + stamina delta via playback ticks / commit model |
| `ApplyStaminaForBridge` | Bridge filler when queue empty | **Drain only** — no recovery bonuses |
| `ApplyProceduralStamina` | Time integration | Applies net rate × duration, clamp 0–100 |
| `chatAutoTickPlaybackStamina` | During playback | Real-time tick aligned to player timeline |

### Two clocks (known tension)

| Clock | Used for | Source |
|-------|----------|--------|
| Wall-clock | `segmentEndsAt`, prefetch throttle, reply publish timing | `time.Now()` |
| HSP timeline | `player.TimelineEndMS() - playheadMS`, append/bridge decisions | Player snapshot |

Implementers must not mix clocks in new code without documenting which authority applies. Feature 001 task 3.2 targets alignment.

### UI snapshot

`GET` chat auto status exposes `stamina` from `state.Stamina`. `prepared.stamina` (projected at segment build) may differ from live tick until commit — UI should prefer live tick when player is running.

### Legacy recover bonus

`ApplyRecover` adds flat bonuses for certain humor/pose combos on gentle segments. Bridge filler (`useRecover=false`) must not apply these bonuses.

## Rationale

Stamina gates autonomous segment length so the LLM roteiro paces tension without fixed timers. Separate commit vs bridge paths prevent infinite recovery during idle filler. Source: `internal/chatauto/stamina.go`, `pose.go`, `internal/httpapi/chat_auto.go`.

## Examples

**Drain at intensity 10 from stamina 50:**

- Rate = 2.0/s → block duration ≈ 25 s until stamina 0

**Recovering intent at stamina 40:**

- Net rate negative → stamina rises toward 100 over segment duration

**Bridge with stamina 98:**

- `ApplyStaminaForBridge` drains only; no `ApplyRecover` bonus

## Edge cases

- `ProceduralBlockDuration` returns minimum 1 second when already at boundary.
- `ProceduralCycleCompleted` fires when stamina crosses 0 (drain) or 100 (recovery) within a block.
- Prefetch uses `chatAutoProjectedStamina()` — may diverge from `state.Stamina` under fast append (P1-G).
- Hybrid chat `intensidade` 1–100 does **not** apply to Chat Auto mapper; do not reuse prompts across modes without scale conversion.

## References

- `internal/chatauto/stamina.go`, `pose.go`, `mapper.go`
- `internal/httpapi/chat_auto.go` — `buildChatAutoSession`, `chatAutoTickPlaybackStamina`
- [`procedural-chat-motion-analysis.md`](../procedural-chat-motion-analysis.md) §14, §18 (P1-G, P1-H)
- Rule 01 (intensity scale difference)
