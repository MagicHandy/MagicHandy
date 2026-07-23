# Rule 01 — Motion Command JSON Contract

## Rule

The LLM must output **exactly one** strict JSON object per assistant turn:

```json
{
  "reply": "<non-empty string>",
  "motion": { ... }
}
```

`motion` is optional. When present, fields must match the active `motion_generation_mode` in settings (`procedural` or `library`). The motion JSON contract appended in code **cannot** be removed from prompt sets.

### Top-level shape

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `reply` | string | Yes | Trimmed; empty rejected |
| `motion` | object | No | Omitted = no motion change |

### Motion actions

| `action` | Meaning |
|----------|---------|
| `none` | No motion change; procedural target fields cleared |
| `start` | Start motion |
| `target` | Retarget while running |
| `stop` | Stop motion |

Unknown actions are rejected at parse time.

### Procedural mode fields (`motion_generation_mode: procedural`)

| Field | Type | Range / values |
|-------|------|----------------|
| `velocidade` | int | 1–100 (defaults to 50 if 0 after normalize) |
| `intensidade` | int or legacy string | 1–100 integer; legacy: `baixa`, `media`, `alta`, `caos` |
| `regiao` | string | See region table below |
| `tipo_batida` | string | See beat type table below |
| `atraso_ms` | int | 0–500 |
| `stroke_range` | `[float, float]` | Scene director only; normalized 0..1 |

**Forbidden in procedural mode:** `library_block_id`, `padrao_id`.

Legacy fallbacks (before validation): `speed_percent` → `velocidade`; semantic `intensidade` string → integer physics.

### Library mode fields (`motion_generation_mode: library`)

| Field | Type | Required when |
|-------|------|---------------|
| `padrao_id` | string | `action` is `start`/`target` (unless `library_block_id` set) |
| `library_block_id` | string | Alternative to `padrao_id` |

**Forbidden in library mode:** `velocidade`, `intensidade`, `regiao`, `tipo_batida`, `pattern_id`, `estilo`, `speed_percent`.

### Valid `regiao` values

`cabeca`, `meio`, `base`, `cabeca_base`, `meio_cabeca`, `meio_base`, `full`, `completo`, `aleatoria`

Invalid values are normalized to `meio_cabeca` at dispatch (not rejected at parse if normalize runs first).

### Valid `tipo_batida` values

`simples`, `leve`, `moderado`, `alto`, `fluido`, `lento`, `very_fast`, `vibrate`, `turbo`

Invalid values normalize to `fluido`.

### Scene director path

When procedural mode is active, responses matching the scene director schema are parsed first (`ParseSceneDirectorResponse`) and converted to `MotionCommand` via `ToMotionCommand(motionRunning)`.

### Repair

Malformed JSON gets **one repair pass** in the chat pipeline; persistent failure surfaces UI indication. Implementers must not add silent third-pass repair without ADR.

## Rationale

Strict JSON prevents partial parses that dispatch half-valid motion. Mode-separated fields stop library IDs from leaking into procedural physics and vice versa. Source: `internal/chat/contract.go`, `chaotic_physics.go`, `scene_director.go`.

## Examples

**Valid procedural start:**

```json
{
  "reply": "Vou acelerar um pouco.",
  "motion": {
    "action": "start",
    "velocidade": 60,
    "intensidade": 70,
    "regiao": "meio_cabeca",
    "tipo_batida": "fluido"
  }
}
```

**Invalid — library field in procedural mode:**

```json
{
  "reply": "...",
  "motion": { "action": "start", "padrao_id": "gentle_wave" }
}
```

→ Error: `procedural motion cannot include library fields`

**Narrative rest — prefer target + lento, not stop:**

Prompts instruct: pauses use `target` + `tipo_batida: lento`, not `stop`. Models emitting `stop` for rest cut the session; automatic repair to `target`/`lento` is **not** implemented yet (see feature 001).

## Edge cases

- `intensidade` accepts integer (1–100) or legacy string in the same JSON key via custom `UnmarshalJSON`.
- `action: none` clears all procedural target fields before validation.
- Chat Auto uses `Intent` with `intensidade` **1–10** scale — different from hybrid procedural 1–100 (see rule 03).
- `pattern_id` (`stroke`, `pulse`, `tease`) is legacy semantic; procedural path prefers physics fields.

## References

- `internal/chat/contract.go`
- `internal/chat/chaotic_physics.go`
- ADR-0002 (semantic vs transport)
- [`procedural-chat-motion-analysis.md`](../procedural-chat-motion-analysis.md) §8
