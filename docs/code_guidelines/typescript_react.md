# TypeScript / React Guidelines — MagicHandy

## Stack

- React 18, TypeScript 5, Vite 5
- Routing: `react-router-dom`
- Animation: `motion` (Framer Motion successor)
- i18n: `i18next` + `react-i18next` (locales in `frontend/src/i18n/locales/`)
- Tests: Vitest + Testing Library

## Structure

```
frontend/src/
  api/          # client.ts, types.ts — mirror backend JSON shapes
  components/   # UI components by feature area
  contexts/     # React context providers
  hooks/        # Reusable stateful logic
  i18n/         # Translations
  lib/          # Pure utilities
  styles/       # CSS modules / global CSS
```

## Components

- Prefer function components with explicit props interfaces
- Colocate feature UI under `components/<feature>/` (e.g. `chat-auto/`)
- Extract hooks when logic is reused or exceeds ~40 lines in a component
- Use `statusSnapshotEqual` and similar helpers to avoid unnecessary re-renders on SSE polls

## API client

- All HTTP calls go through `frontend/src/api/client.ts`
- Types in `api/types.ts` must match Go JSON field names (`snake_case` from backend)
- Handle `{ error: string }` responses for non-streaming routes
- SSE streams: parse events incrementally; surface malformed LLM state in UI

## Styling

- Follow [`docs/ui-design-guidelines.md`](../ui-design-guidelines.md)
- CSS split: `shell.css`, `layout.css`, `polish.css` — extend existing files before adding new globals
- Prefer CSS variables from the design system over hard-coded colors

## Build and embed

```powershell
cd frontend
npm run build   # outputs to uibuild/dist via vite config
```

Go embeds `uibuild/dist`; never edit embedded files directly.

## Testing

```powershell
cd frontend
npm run test
```

- Test pure functions in `lib/` with Vitest
- Component tests for critical panels (status, session controls) when behavior is non-trivial
- Mock `fetch` / API client for component tests; no live backend in unit tests

## i18n

- User-facing strings in locale JSON, not hard-coded in components (except dev-only labels)
- Keep `pt.json` and `en.json` keys in sync when adding strings
