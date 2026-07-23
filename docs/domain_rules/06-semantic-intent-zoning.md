# Rule 06 — Semantic Intent Enums and Legacy Regiao Map

## Rule

When **Director mode** is enabled (`settings.llm.director_mode`), the LLM returns **enums only** (`action`, `location`, `intensity`). Go resolves stroke bounds via `semantic.ResolveMotionBounds` — the model must not output stroke math.

Legacy procedural chat (director off) continues using `MotionCommand` with `regiao` strings until fully migrated.

## Director enums

### Actions

`oral`, `handjob`, `riding`, `titjob`, `deepthroat`

### Locations

`base`, `shaft`, `tip`, `full`

### Intensity

Integer `1..10`

## Legacy regiao ↔ location map

| Legacy `regiao` | Director `location` | Default zone (0..1) |
|-----------------|---------------------|------------------------|
| `base`, `meio_base` | `base` | 0.0–0.3 |
| `meio`, `meio_cabeca` | `shaft` | 0.3–0.7 |
| `cabeca` | `tip` | 0.7–1.0 |
| `full`, `completo`, `cabeca_base`, `aleatoria` | `full` | 0.0–1.0 |

Implemented in `semantic.RegiaoToLocation` / `semantic.LocationToRegiao`.

## Action overrides (defaults)

| Action | Forced zone |
|--------|-------------|
| `oral` | `full` |
| `deepthroat` | `full` |

User `motion_preferences.action_overrides` can customize.

## SSE contract (director mode)

Event order: `status` → `intent` → `motion` → `delta`* → `message` → `done`

`intent` payload: `{action, location, intensity}`

## Examples

- Director returns `{action: "handjob", location: "tip", intensity: 7}` → strokes confined to tip band (default 0.7–1.0).
- Legacy JSON `{regiao: "meio_cabeca"}` → maps to `shaft` when bridging to director prefs.

## Edge cases

- Unknown enum → director repair pass; still invalid → HTTP error event.
- Custom tip zone `0.75–0.95` in settings honored for non-overridden actions.

## Related

- Feature 002 phases 1–3
- Rule 01 (motion JSON contract)
- ADR-0015
