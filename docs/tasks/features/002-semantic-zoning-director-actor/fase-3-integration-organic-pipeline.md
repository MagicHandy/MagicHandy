# Phase 3 — End-to-End Integration + Device Validation

## Objective

Wire semantic resolver + Director/Actor + bridge filler into production procedural path. Validate TTFP and uninterrupted motion on real device (Rule 04).

## Prerequisites

- Phases 0–2 complete
- `TestProceduralSyncUninterruptedOnRealDevice` passing as regression gate

---

## Task 3.1 — Wire resolver into motion dispatch

**Action:**

1. In `playChatChaoticMotion`, when director mode active:
   - Build `OrganicConfig` via `OrganicConfigFromIntent` instead of `ChaoticPhysics.Regiao` alone
2. Pass `StrokeMin`/`StrokeMax` into `buildChaosSession*` (already via `physics.StrokeRange` or extend `ChaoticPhysics`)
3. Keep legacy path when director mode off

**Acceptance criteria:**

- Device test: `location: tip` confines position samples to tip band (±5%)
- Traces show resolved bounds

---

## Task 3.2 — Settings + migration

**Action:**

- `settings.Motion.MotionPreferences` JSON blob
- `settings.LLM.DirectorMode bool` default false
- Settings UI stub (optional): zones editor deferred

**Acceptance criteria:**

- Factory reset restores defaults
- API redaction does not expose raw prefs if sensitive

---

## Task 3.3 — Device validation suite

**Action:**

1. `TestDirectorActorTTFPDevice` (integration tag):
   - Send chat message with director mode on
   - Measure time from HTTP POST to first `intent` SSE + first HSP add
   - Assert TTFP < 500ms + cloud latency (document actual in log)
2. Extend sync test for 20s director-mode session with bridge filler

**Acceptance criteria:**

- Rule 04 session log in TRACKER
- No starvation events; `hsp_plays` minimal restarts

---

## Task 3.4 — Documentation

**Action:**

- ADR-0015 (proposed): Director/Actor chat latency architecture
- Domain rules 05 (bridge), 06 (intent enums + legacy regiao map)
- Update `procedural-chat-motion-analysis.md` §1 stack diagram

**Acceptance criteria:**

- `docs/adrs/README.md` index updated
- `AI-GUIDELINE.md` feature table lists 002

---

## Example flow (reference)

```go
intent, err := AskDirector(ctx, provider, userMsg, history)
prefs, _ := loadMotionPreferences(settings)
min, max, _ := semantic.ResolveMotionBounds(intent, prefs)
cfg, _ := semantic.OrganicConfigFromIntent(intent, prefs, velocityFromIntensity(intent.Intensity))
physics := motion.ChaoticPhysics{
    Velocidade: velocityFromIntensity(intent.Intensity),
    Intensidade: intent.Intensity * 10,
    StrokeRangeMin: min,
    StrokeRangeMax: max,
    Regiao: semantic.LocationToRegiao(intent.Location), // compat
}
// → buildChaosSession → player.Start
go AskActor(ctx, provider, userMsg, history, intent, streamTokens)
```

Until new message or stamina/event changes zone, bounds remain latched on `chatChaosRuntime`.

---

## Phase completion

- Feature 002 TRACKER all `[x]`
- Central TRACKER updated
- Recommend follow-up: stamina-driven zone shifts (Chat Auto parity)
