# Character Card Import Design

Date: 2026-07-30
Status: approved for implementation (autonomous session; assumptions listed below)

## Goal

Import roleplay character cards into the existing persona system, from three
sources: Tavern-style PNG cards with an embedded `chara`/`ccv3` payload,
plain JSON card files, and URLs to sites that publish card data. A card
becomes a persona: portrait, name, description, lore (personality and
scenario), and a greeting that opens a new chat.

## What already exists (and is reused, not duplicated)

Personas on main already carry a portrait (JPEG ≤ 2 MiB, ≤ 1024 px), a name
(≤ 60 chars), a description (≤ 500 chars, quoted into the prompt as data),
bounded lore entries (≤ 8 entries, ≤ 500 chars each, ≤ 2000 total, composed
into the prompt with an existing budget), a per-session persona assignment
(`PUT /api/chat/sessions/{id}/persona`), and a portable `.mhpersona`
import/export. Prompt composition needs no changes: card fields ride the
existing description and lore paths. The one missing concept is a greeting.

## Assumptions (made without live user input)

- Cards map onto personas rather than a parallel "characters" system.
- Card `system_prompt` / `post_history_instructions` are ignored: every
  card field enters the prompt as bounded, quoted data through the existing
  description/lore paths, per the repo rule that the contract is code, not
  prompt text.
- Card text that exceeds the persona/lore bounds is truncated, and the
  import response reports what was truncated. The bounds are the app's
  deliberate motion-adherence budget; import respects them.
- `{{char}}` is replaced with the character name; `{{user}}'s` with "your"
  and `{{user}}` with "you" (case-insensitive) at import time.
- synsual.me exposes character data only to logged-in accounts (verified:
  every relevant API route requires a bearer token, and the page is an SPA
  shell). URL import therefore targets sites that publish card files or
  embed card JSON publicly, and returns a clear "requires login" style
  error otherwise. chub.ai is geo-blocked from the build machine, so no
  site-specific adapters are shipped; the generic engine covers direct
  PNG/JSON links and pages with discoverable card data.

## Components

### internal/charcard (new package, stdlib only, no internal imports)

- `Card{Name, Description, Personality, Scenario, Greeting, ExampleMessages,
  CreatorNotes}` — normalized from V1 (flat), V2/V3 (`data` object) JSON.
- `ParsePNG(data)` — walks PNG chunks with encoding/binary, reads the
  `tEXt` payload keyed `ccv3` (preferred) or `chara`, base64-decodes,
  normalizes. `ParseJSON(data)`, and `Parse(data)` which sniffs PNG magic.
- `ReplaceMacros(text, name)` — the substitution rules above.
- `fetch.go`: `Fetch(ctx, client, url)` — https/http only, response capped
  (16 MiB), sniffs PNG/JSON directly; for HTML it (a) scans script bodies
  for embedded card JSON (`"spec":"chara_card_v2|v3"` or first_mes-bearing
  objects), (b) follows at most one same-host link that looks like a card
  file (.png/.json), (c) uses og:image for the portrait when card JSON was
  found without art. Returns Card + optional portrait PNG bytes, or a
  typed "no card data found" error.

### internal/persona additions

- Store migration v18: `personas.greeting TEXT NOT NULL DEFAULT ''`
  (guarded hook, same pattern as v16/v17), registered in schema
  validation. Bound: ≤ 2000 chars.
- `Persona.Greeting` + `Draft.Greeting`, validated and editable like other
  fields; carried in the portable archive (schema stays v1-compatible:
  greeting is additive and optional).
- `cardimport.go`: `ImportCard(ctx, store, card, portraitPNG)` — creates a
  persona: name, description (truncated), lore entries in priority order
  (personality, scenario, description overflow, example messages) within
  existing budgets, greeting (macros replaced), and portrait: PNG decoded,
  downscaled to ≤ 1024 px with a small pure-Go scaler, re-encoded JPEG.
  Returns the persona plus human-readable truncation warnings.

### internal/httpapi

- `POST /api/personas/import` learns to sniff its body: ZIP → existing
  archive path; PNG or JSON → card path. Response gains `warnings`.
- `POST /api/personas/import-url` `{url}` (controller-gated): fetch via
  charcard, then the same card path. Maps fetch failures to 502-style
  errors with translatable messages.
- Greeting seeding: when `PUT /api/chat/sessions/{id}/persona` attaches a
  persona with a greeting to a session that has no messages, the greeting
  is appended as the first assistant message.

## Web UI

- Personas page import affordance accepts `.mhpersona,.png,.json`, plus an
  "Import from URL" control calling the new endpoint; truncation warnings
  are surfaced once after import.
- Persona editor gains a Greeting textarea.
- New strings via `t(...)` in all five locale catalogs.

## Error handling

- PNG without card chunk, bad base64/JSON, missing name → 422 with a
  translatable message. Oversized body → 413. Library full → 409 (existing).
- URL import: network failure/timeout → 502; page with no discoverable
  card data → 422 with guidance to download the card file and import it.

## Testing

- charcard: synthetic PNG fixtures built in-test (not the third-party
  cards in chars/), V1/V2/V3 normalization, macros, HTML extraction, fetch
  via httptest server.
- persona: card→persona mapping bounds and warnings, greeting column
  round-trip, portrait conversion.
- httpapi: import sniffing (zip/png/json), import-url, greeting seeding on
  persona attach, error mapping.
- Manual: chars/Annabelle.png and chars/Lily.png import end to end.
