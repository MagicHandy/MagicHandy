# React UI Implementation Handoff

> **Historical (consumed).** This handoff drove the React migration that
> merged as PRs #37/#38 (2026-07-08); the implementation sequence below is
> complete and the non-negotiables it lists are now enforced by tests and
> CI rather than by this document. Kept for the record; for current UI
> guidance see [ui-design.md](ui-design.md),
> [ui-navigation-redesign.md](ui-navigation-redesign.md) (as-built shell),
> [ui-design-guidelines.md](ui-design-guidelines.md) (live tokens), and
> [settings-compaction.md](settings-compaction.md) (next planned change).

## Goal

Implement the UI design docs in React without changing MagicHandy's safety model
or release shape.

The target is the sidebar-navigation shell from:

- `docs/ui-navigation-redesign.md`
- `docs/ui-design-guidelines.md`
- `docs/ui-design.md`

The architecture decision is `docs/decisions/0009-react-frontend.md`.

## Non-Negotiables

- Runtime must still be a Go binary serving embedded static assets. No runtime
  Node process, Vite dev server, external font fetch, icon CDN, or remote asset.
- Backend state remains authoritative. Do not duplicate motion, transport,
  settings, controller ownership, prompt/memory, or diagnostics state as a
  separate client model.
- Stop is always mounted outside the active route and remains available to
  read-only clients and backend-offline attempts.
- The frontend never constructs raw Handy transport payloads.
- Keep the visualizer as an engine-state component with commanded-estimate
  labeling; no guessed client-side motion.
- Implement the visual language from `docs/ui-design-guidelines.md`: compact
  dot+text status readouts, no oversized round bubbles, one steel-azure
  interactive hue, green only for go/running, red for Stop.

## Recommended Stack

- Vite + React + TypeScript.
- Plain CSS files using existing custom-property tokens. Avoid CSS-in-JS.
- Inline SVG icon components using `currentColor`; no emoji UI icons.
- Vitest + Testing Library for component tests.

Use React normally and simply. Do not add broad `useMemo`/`useCallback` wrappers
by default. Use `startTransition`, `useDeferredValue`, or effect-event style
patterns only where the interaction needs them: streaming chat, route changes,
large pattern lists, or SSE/event handlers that otherwise capture stale state.

## Suggested File Layout

```text
web/
  package.json
  package-lock.json
  vite.config.ts
  tsconfig.json
  index.html
  src/
    main.tsx
    App.tsx
    api/
      client.ts
      types.ts
    state/
      app-state.tsx
      controller.tsx
      motion-events.ts
    shell/
      AppShell.tsx
      NavRail.tsx
      StatusBar.tsx
      StopButton.tsx
      ToastHost.tsx
    routes/
      ChatRoute.tsx
      PresetModesRoute.tsx
      PatternLibraryRoute.tsx
      SettingsRoute.tsx
    components/
      MotionVisualizer.tsx
      ChatPanel.tsx
      QuickSettings.tsx
      ManualMotionTest.tsx
      PromptSetEditor.tsx
      MemoryManager.tsx
      DiagnosticsPanel.tsx
    styles/
      tokens.css
      shell.css
      components.css
```

This layout is a guide, not a requirement. Keep files small and owned by one
surface. Split before creating another `app.js`-style coordinator.

## Data/API Layer

Create typed wrappers around the existing API before building components:

- `GET /healthz`
- `GET /api/state`
- `GET /api/transport/bluetooth/status`
- motion commands: start/stop/pause/resume, quick settings, trace export
- settings save/reset
- prompt sets CRUD
- memory CRUD/toggles
- chat stream
- LLM check/load/unload
- connection check

Types in `src/api/types.ts` should mirror Go JSON payloads closely. Keep string
unions for known enum-like values such as motion state, dispatch owner, provider,
transport state, and controller role.

## State Ownership

Use a small number of providers/hooks:

- `AppStateProvider`: latest `/api/state`, `/healthz`, backend availability,
  stale/error state.
- `ControllerProvider`: client ID, read-only status, controller lock reason.
- `MotionEvents`: SSE updates and fallback polling. The backend snapshot remains
  authoritative.
- Route-local state: input drafts, selected editor row, confirmation arm timers,
  dirty form flags.

Avoid one global store that every component mutates. If a component cannot state
its backend source or local-only reason, it probably should not own that state.

## Implementation Sequence For Claude

1. **Scaffold React without changing behavior.** Add Vite/React/TypeScript,
   build to static output, embed that output from Go, and make CI run npm build
   plus Go tests. A placeholder shell is acceptable only if it preserves visible
   Stop and backend status.
2. **Port existing behavior into React components.** Preserve current APIs and
   behavior: status, backend-loss lock, controller read-only lock, Stop/Escape,
   visualizer, quick settings, manual motion test, chat streaming, settings,
   prompt sets, memory, diagnostics.
3. **Apply the sidebar-navigation shell.** Implement routes `#/chat`, `#/modes`,
   `#/library`, and `#/settings/*`; move Stop to the permanent nav footer; move
   current quick controls into the Chat route; convert Settings from overlay to
   route.
4. **Add Preset Modes placeholder/first implementation.** Relocate Freestyle and
   style selection. Add Autopilot UI only when the backend contract exists; until
   then render disabled/coming-soon with a clear reason.
5. **Add Pattern Library.** The interim empty state shipped with the shell and
   was replaced in Phase 14 by Browse, Programs, Author, and Training backed by
   the canonical library API.
6. **Retire legacy files only after parity tests pass.** Remove old vanilla JS
   assets from embedding once React tests cover their safety-critical behavior.

## Acceptance Checklist

The PR is not done until all are true:

- `go run ./cmd/magichandy` serves the built React app without Node running.
- `CGO_ENABLED=0 go build ./cmd/magichandy` still passes.
- Stop is visible and reachable on every route and not inside the status bar.
- Extra/read-only clients can still Stop but cannot send ordinary motion.
- Backend loss shows a persistent banner and disables backend-required controls.
- The status bar uses compact readouts, not `.status-pill`-style round bubbles.
- Chat keeps near-bottom scroll behavior and jump-to-latest.
- Settings are routed pages with deep links, not stacked overlays.
- Existing prompt/memory/reset behavior is preserved.
- No component imports or calls low-level transport commands.
- Tests assert the route shell, Stop placement, backend lock, read-only lock,
  chat stream handling, and settings/prompt/memory critical paths.

## Suggested Claude Prompt

```text
Implement the React UI migration for MagicHandy.

Read these first: docs/decisions/0009-react-frontend.md,
docs/react-ui-implementation-handoff.md, docs/ui-navigation-redesign.md,
docs/ui-design-guidelines.md, docs/ui-design.md, and web/assets_test.go.

Goal: migrate the browser UI to a static Vite + React + TypeScript app embedded
by Go, then implement the sidebar-navigation shell. Preserve existing APIs and
safety behavior. Runtime must remain a Go binary with embedded assets; Node is
only for development/CI build.

Do not duplicate backend motion/controller state in React. Stop must stay
always mounted outside route content and available to read-only clients. The
frontend must never construct raw Handy transport payloads.

Add npm build/test commands, update Go embedding/tests for built assets, and run
the full validation listed in docs/react-ui-implementation-handoff.md.
```
