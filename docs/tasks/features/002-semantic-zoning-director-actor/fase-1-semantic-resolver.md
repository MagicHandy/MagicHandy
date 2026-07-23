# Phase 1 — Semantic Resolver (Zoning + Actions)

## Objective

LLM returns **enums only**; Go resolves `[minStroke, maxStroke]` from user preferences and action compatibility overrides. Output feeds `OrganicConfig` directly.

## Prerequisites

- Phase 0 optional but recommended for continuity testing
- Read `motion.OrganicConfig`, `OrganicConfigFromPhysics`, `motion.RegionBounds`
- ADR-0002 semantic vs transport

## Package layout (proposed)

```
internal/motion/semantic/
  preferences.go    # MotionPreferences + defaults + SQLite load/save hooks
  intent.go         # LLMIntent + enum validation
  resolver.go       # ResolveMotionBounds
  resolver_test.go  # table-driven matrix
  organic_adapter.go # OrganicConfigFromIntent
```

Persist `MotionPreferences` via existing `internal/store` settings document extension (avoid new table until needed).

---

## Task 1.1 — MotionPreferences

**Action:**

```go
type ZoneName string
const (
    ZoneBase  ZoneName = "base"
    ZoneShaft ZoneName = "shaft"
    ZoneTip   ZoneName = "tip"
    ZoneFull  ZoneName = "full"
)

type ZoneRange struct {
    Min float64 // 0..1 normalized
    Max float64
}

type MotionPreferences struct {
    Zones map[ZoneName]ZoneRange
    // ActionOverrides force zone regardless of Location (realism).
    ActionOverrides map[ActionName]ZoneName
}
```

Defaults: Tip 0.7–1.0, Base 0.0–0.3, Shaft 0.3–0.7, Full 0.0–1.0.

**Acceptance criteria:**

- `DefaultMotionPreferences()` returns documented defaults
- JSON round-trip through settings blob
- Migration: missing fields → defaults

---

## Task 1.2 — LLMIntent contract

**Action:**

```go
type ActionName string
const (
    ActionOral       ActionName = "oral"
    ActionHandjob    ActionName = "handjob"
    ActionRiding     ActionName = "riding"
    ActionTitjob     ActionName = "titjob"
    ActionDeepthroat ActionName = "deepthroat"
)

type LocationName string
const (
    LocationBase  LocationName = "base"
    LocationShaft LocationName = "shaft"
    LocationTip   LocationName = "tip"
    LocationFull  LocationName = "full"
)

type LLMIntent struct {
    Action    ActionName   `json:"action"`
    Location  LocationName `json:"location"`
    Intensity int          `json:"intensity"` // 1–10
}
```

`ValidateLLMIntent(intent) error` — strict enums, intensity 1–10.

**Acceptance criteria:**

- Unknown action/location rejected
- Tests for each valid enum

---

## Task 1.3 — ResolveMotionBounds

**Action:**

```go
func ResolveMotionBounds(intent LLMIntent, prefs MotionPreferences) (min, max float64, err error)
```

Logic:

1. If `prefs.ActionOverrides[intent.Action]` exists → use that zone range
2. Else map `intent.Location` → `prefs.Zones`
3. Deepthroat/oral override → `ZoneFull` (configurable in ActionOverrides)
4. Clamp 0..1; return error if zone missing

**Acceptance criteria:**

- Table test: deepthroat + location tip → full range
- Table test: handjob + tip → tip range
- Table test: custom user tip 0.75–0.95 honored

---

## Task 1.4 — Unit test matrix

**Action:** `resolver_test.go` with ≥12 cases covering overrides, defaults, invalid enums, custom prefs.

**Acceptance criteria:** `go test ./internal/motion/semantic/...`

---

## Task 1.5 — OrganicConfig adapter

**Action:**

```go
func OrganicConfigFromIntent(intent LLMIntent, prefs MotionPreferences, velocity int) (OrganicConfig, error) {
    min, max, err := ResolveMotionBounds(intent, prefs)
    // min,max are 0..1 → multiply to 0..100 for StrokeMin/StrokeMax
    return OrganicConfig{
        StrokeMin: min * 100,
        StrokeMax: max * 100,
        BaseVelocity: float64(velocity),
        Intensity: float64(intent.Intensity * 10), // scale to 0..100
        // NoiseWeight, Asymmetry from existing OrganicConfigFromPhysics heuristics
    }, nil
}
```

**Acceptance criteria:**

- Example in godoc: `location: tip` → waves only 70–100%
- Test asserts bounds match resolver output

---

## Task 1.6 — Legacy compat mapping

**Action:**

- Map existing `regiao` strings (`meio_cabeca`, etc.) ↔ `LocationName` for transition period
- `RegiaoToLocation(regiao string) LocationName` used when `LLMIntent` not yet deployed

**Acceptance criteria:**

- Existing chat JSON still works until Director mode enabled
- Document mapping table in domain rule 06 (created in phase 3)

---

## Phase completion

- No HTTP handler changes yet — resolver is pure Go + tests
- Update feature 002 TRACKER
