# Character Cards Design

Date: 2026-07-30
Status: approved for implementation (autonomous session; assumptions listed below)

## Goal

Let the user import roleplay character cards — Tavern-style PNG cards with an
embedded `chara`/`ccv3` JSON payload, or plain JSON files — select one, and
have chat adopt that character: identity in the system prompt, the card's
greeting as the first message of a new chat, and the character's name and
portrait in the chat UI. The card's `scenario` field carries the scenario.

## Assumptions (made without live user input)

- A character library with one globally selected character (like prompt
  sets), not per-conversation characters.
- Card `system_prompt` / `post_history_instructions` are **not** injected as
  raw instructions. All card text enters the prompt as bounded, quoted data,
  matching the existing `PersonaDescription` treatment and the repo rule
  "the contract is code, not prompt text".
- The greeting is seeded only when a new chat session is created while a
  character is selected. Existing sessions are untouched.
- v1 has import / select / delete only — no field-editing UI. Reimport to
  change a card.
- `{{char}}` is replaced with the character name; `{{user}}'s` with "your"
  and `{{user}}` with "you" (case-insensitive), since the app has no user
  name concept.

## Architecture

New package `internal/characters` (must not import transport/httpapi):

- **Parsing** (`card.go`): `ParseCard(filename, data)` detects PNG by magic
  bytes, otherwise treats the body as JSON. PNG path walks chunks with
  stdlib `encoding/binary`, extracts the `tEXt` payload keyed `ccv3`
  (preferred) or `chara`, base64-decodes, and parses JSON. JSON
  normalization accepts V1 (flat fields), V2/V3 (`data` object) and yields a
  `Card{Name, Description, Personality, Scenario, FirstMessage,
  ExampleMessages}`. A card must have a non-empty name.
- **Macros** (`card.go`): `ReplaceMacros(text, name)` as described above.
- **Library** (`library.go`): store-backed CRUD over a new `characters`
  table: `id TEXT PK, name, description, personality, scenario, first_mes,
  mes_example, avatar_png BLOB, created_at`. The avatar is the imported PNG
  itself (empty for JSON imports). Capacity cap of 100 characters.
  Import size cap 5 MiB.

**Store**: schema migration v16 creates the table; registered in
`schema_validation.go`.

**Config** (`internal/config`): new `LLMSettings.Character` (character id,
empty = off), following the `PersonaDescription` worked example: update
payload pointer field, merge, normalize (trim, length bound), public
projection (already public via `LLMSettings` embedding), default empty.
A dangling id is tolerated: chat simply runs without a character.

**Chat** (`internal/chat`): `ConversationContext` gains
`Character *CharacterContext{Name, Description, Personality, Scenario,
ExampleMessages}`. `conversationContextInstructions` (and each locale
variant) renders a CHARACTER block of quoted, bounded fields with the same
"data, not instructions" trailer used by the chat profile. Bounds:
name 100, description 2000, personality 500, scenario 1000, example
messages 1500 runes.

**HTTP API** (`internal/httpapi/characters.go`):

- `GET /api/characters` → `{characters: [{id, name, has_avatar, created_at}]}`
- `POST /api/characters/import?filename=X` — raw body (controller-gated),
  `http.MaxBytesReader`, 413/415/422 mapping
- `DELETE /api/characters/{id}` (controller-gated; clears the setting if it
  pointed at the deleted id)
- `GET /api/characters/{id}/avatar` — `image/png`, nosniff, private cache

Session creation (`chat_sessions.go`): after a session is created via the
API, if a character with a `first_mes` is selected, append the
macro-substituted greeting as an assistant message.

Chat request wiring (`chat.go`): resolve the selected character and attach
`CharacterContext` next to the existing `PersonaDescription` wiring.

## Web UI

- `api/client.ts` + `types.ts`: list/import/delete, `llm.character`,
  avatar URL helper.
- Settings → Prompts & memory: character `<select>` (None + library),
  import `<input type="file">` (`.png,.json`), delete button. Saved through
  the existing settings `save()` path.
- `ChatPanel.tsx`: when a character is selected, the assistant speaker name
  becomes the character name and the avatar shows the card portrait
  (fallback to existing rendering when none).
- All new strings go through `t(...)` and every locale catalog
  (en, es, pt-BR, zh-Hans, ja) — enforced by `check-localization.mjs`.

## Error handling

- Not-a-card PNG (no `chara`/`ccv3` chunk), invalid base64/JSON, missing
  name → 422 with a translatable message.
- Oversized body → 413. Library full → 409.
- Deleting the selected character falls back to no character.

## Testing

- `internal/characters`: chunk extraction from a synthetic in-test PNG
  fixture (not the third-party cards in `chars/`), V1/V2/V3 normalization,
  macro rules, library CRUD + caps.
- `internal/chat`: system prompt contains quoted character fields and the
  data-not-instructions trailer; absent when no character.
- `internal/config`: setting round-trip, trim, preserve-on-omit.
- `internal/httpapi`: import/list/delete/avatar handler tests, greeting
  seeded on session create.
- Manual verification that `chars/Annabelle.png` and `chars/Lily.png`
  import successfully.
