# Persona Page And Long-Context Design

Design for a routed **Personas** page — a tile library with a "New persona"
first tile, each persona carrying a portrait, name, and description — plus a
dispositioned set of ideas for giving the model deeper "lore" and longer
conversational context without degrading motion control.

**Status: sections 1–6 shipped 2026-07-29.** Section 7 remains a ranked idea
catalog in the style of [feature-ideas.md](feature-ideas.md); nothing in it is
scheduled, and lore is deliberately not built (see §6).

[persona-page-sketch.svg](persona-page-sketch.svg) is the layout: the rail with
its new entry, the tile grid with its leading create tile, and the editor drawer
open on the lore budget and prompt-composition meter.

---

## 0. What the reference shape is, and what it is not

The requested interface is the familiar character-card grid: full-bleed
portrait tiles, name and truncated description over a gradient scrim, small
chips in the corners, and a create affordance in the first cell.

That shape is right. Two thirds of its *content* is not, because the reference
is a **catalog of other people's characters** and this is a local app:

| In the reference | Here |
| --- | --- |
| `@author` handle | Nothing. Every persona is the user's own. |
| View counts (`177`, `70`) | Last used, or nothing. Popularity is meaningless with one user. |
| `FREE` / paid chips | Nothing. |
| `XXX` / `NSFW` chips | The persona's **reply register** (`utility` / `warm` / `intimate` / `explicit`) — the same fact, but it *does* something: it is the actual axis value the prompt composes. |
| `Stroker` / `Vibrators` / `Prostate Toys` device chips | Nothing device-shaped. MagicHandy drives one linear actuator (ADR 0002); a device taxonomy would be fiction. |
| `+ Memory` chip | Real, and worth keeping: whether the persona has **lore entries** of its own (section 7). |

So the chips stay, but they report local state — active, register, lore count,
missing portrait — rather than marketplace furniture. Copying the reference's
chip *set* would be cargo cult; copying its *density and legibility* is the
point.

---

## 1. Where a persona fits in what already exists

This is the part that decides whether the feature is cheap or a rewrite.
MagicHandy does **not** have a free-text persona blob. It composes the system
prompt from independent, code-owned, individually validated axes:

| Axis | Setting today | Owner |
| --- | --- | --- |
| Behavior profile | `llm.prompt_set` (+ `prompt_sets` table) | Editable text, contract appended in code |
| Reply register | `llm.chat_voice` | Code-owned enum: utility / warm / intimate / explicit |
| Partner identity | `llm.persona_description` (≤500 chars) | Bounded free text, quoted as JSON data |
| User anatomy | `llm.user_anatomy` + `custom_anatomy` (≤120) | Code-owned vocabulary |
| Mood | model-reported `new_mood` | 17-value enum, per session, no motion effect |
| Motion capability | `llm.motion_capabilities` | Checkbox gates; disabled methods are never described and stripped if emitted |

[lso-merge-alternatives.md §Decision 3](lso-merge-alternatives.md) already
settled what a persona feature should therefore be:

> Presets (saveable named combinations of the axes above) are the useful part of
> LSO's persona model and can be added later without changing this composition.

**A persona is a named, portrait-bearing preset over those axes — plus one new
axis and one new content type.** It is not a second personalization system, and
it is not a prompt fragment the user injects. Composition stays
`composeSystem()` in [internal/chat/prompts.go](../internal/chat/prompts.go);
the page is a surface over values it already consumes.

Two additions the axes do not cover today:

- **Display name.** STGPT-RV had `ai_name`; MagicHandy has no name for the
  assistant at all. It is the one field a persona page cannot work without, and
  it is listed as an unscoped gap in
  [feature-ideas.md §A](feature-ideas.md) ("AI display name, profile picture,
  splash personalization — worth scoping, cosmetic tier").
- **Reaction style.** The documented gap: submissive / dominant / playful /
  teasing is orthogonal to how explicit the language is, and is not expressible
  today. Add it the same way `chat_voice` was added — a code-owned enum with a
  composed instruction block, never user text.

---

## 2. Data model

### 2.1 Schema — migration v15 → v16

```sql
CREATE TABLE IF NOT EXISTS personas (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    description       TEXT NOT NULL DEFAULT '',
    chat_voice        TEXT NOT NULL DEFAULT 'warm',
    reaction_style    TEXT NOT NULL DEFAULT 'neutral',
    prompt_set_id     TEXT NOT NULL DEFAULT '',
    tts_voice_id      TEXT NOT NULL DEFAULT '',
    preferred_tags_json TEXT NOT NULL DEFAULT '[]',
    default_focus_area  TEXT NOT NULL DEFAULT 'full',
    lore              TEXT NOT NULL DEFAULT '',
    portrait_updated_at TEXT NOT NULL DEFAULT '',
    last_used_at      TEXT NOT NULL DEFAULT '',
    created_at        TEXT NOT NULL,
    updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS personas_used ON personas(last_used_at DESC, name);

ALTER TABLE chat_sessions ADD COLUMN persona_id TEXT NOT NULL DEFAULT '';
```

`CHECK` constraints are deliberately **not** used for the enums. The existing
tables (`patterns.origin`, `messages.role`) use them because those value sets
are structural; `chat_voice` and `reaction_style` are product vocabulary that
will gain values, and a `CHECK` turns each addition into a table rebuild.
Validation lives in Go beside the existing `chat_voice` validation, which is
where the option lists are already single-sourced
(`settings_public_options.go`).

`persona_id` on `chat_sessions` is `''`-defaulted rather than a foreign key, and
persona deletion does **not** cascade: an old session should keep reading as it
was written even after its persona is gone. The chat header resolves a missing
persona to the global axis values and says so.

Bounds, matching the existing style (`MaxLLMPersonaDescriptionChars = 500`,
`maxPromptNameChars = 80`):

| Field | Bound | Why |
| --- | --- | --- |
| `name` | 60 runes | Tile-legible; longer names truncate rather than wrap to three lines |
| `description` | 500 runes | Reuses the validated `persona_description` bound and its prompt path verbatim |
| `lore` | 2000 runes | See section 7; enters the prompt only under a budget |
| personas | 200 rows | Same ceiling as memories |

### 2.2 Portraits

Reuse the media thumbnail store's shape exactly — it already solves the two
hard parts (path traversal and atomic replace):

- Path `<dataDir>/personas/<id>.jpg`, resolved through the same
  identifier-validation-then-join used by
  [`thumbnailPath`](../internal/media/thumbnails.go) so a hostile ID cannot
  escape the directory.
- Written to `<id>.jpg.partial` and renamed, as thumbnails are.
- `portrait_updated_at` doubles as existence flag and cache-buster in the tile
  URL (`?v=<timestamp>`).
- Purgeable, and listed in the Diagnostics disk-usage idea alongside
  thumbnails, models, and voice trees.

**Resize in the browser, not the server.** The client already captures video
frames to a canvas and POSTs a JPEG blob for media thumbnails; portraits use
that same path — draw the chosen file to a canvas at max edge 512, export
`image/jpeg` at 0.85, POST. The server validates *decodability, dimensions, and
byte ceiling* with `image/jpeg` from the standard library and nothing else. The
alternative — server-side scaling — needs a new image-scaling dependency, or
FFmpeg, and FFmpeg is explicitly optional
([media-tooling.md](media-tooling.md)). A persona portrait must not be the
first feature that makes it mandatory.

Accepting PNG uploads is handled the same way: the browser decodes anything
`<img>` can decode and always uploads JPEG, so the server contract stays
single-format.

### 2.3 The new reaction-style axis

Composed in code exactly like `voiceIdentityInstructions` /
`finalVoiceCheck` — a `switch` over a code-owned enum returning a fixed string,
inserted in the same two positions, never interpolating user text:

| Value | Shapes |
| --- | --- |
| `neutral` | Nothing (the default; composes no block at all) |
| `playful` | Teasing, light, initiates jokes |
| `tender` | Attentive, reassuring, slow |
| `dominant` | Leads, directs, states what happens next |
| `submissive` | Follows, asks, defers |
| `teasing` | Withholds, draws out, denies briefly |

Two invariants for the block, both testable:

1. It never mentions motion, speed, patterns, areas, JSON, or the device. A
   `dominant` persona must not be able to imply authority over the actuator.
   The existing `motion_authorization_test.go` pattern is the model for pinning
   this.
2. `neutral` composing *nothing* means the feature is inert by default, so an
   existing user's prompt is byte-identical until they opt in. Worth a test:
   `ComposeSystem` output for a `neutral` persona equals today's output.

---

## 3. What a persona may and may never carry

The table is the point of the design. Everything on the right stays global and
user-owned, because a picture with a name must never be a way to change a
safety boundary.

| A persona **may** carry | A persona **may never** carry |
| --- | --- |
| Display name, portrait, description | The motion JSON contract (code-appended, Phase 10 rule) |
| Reply register (`chat_voice`) | Motion capability gates (`motion_capabilities`) |
| Reaction style | Speed, depth, or acceleration limits |
| Behavior profile (`prompt_set_id`) | Focus-range bounds |
| TTS voice selection | User anatomy — that describes *the user*, and switching partners must not silently rewrite the user's own body |
| Preferred pattern tags (a *hint* into existing weighting) | Pattern enable/disable state |
| Default focus area (within the user's configured focus range) | Autopilot arming, autospeak, or any autonomy level |
| Lore entries | Global long-term memories |

The two entries most likely to be argued:

- **Preferred pattern tags** are a preference, not an authorization. They enter
  the prompt through the existing `preference_weight` in
  `curationInstructions` — a persona nudges ordering among *already enabled*
  patterns and can neither enable one nor invent an ID. If that plumbing proves
  fiddly, ship personas without it; it is the most droppable field here.
- **Default focus area** is a starting value for `area`, clamped by the user's
  configured focus range like any model-requested zone. It cannot widen travel.

**Selecting a persona is not a settings write.** It sets the session's
`persona_id`. The global axis values in Settings stay exactly as the user left
them and remain the fallback when no persona is active — so a user who never
opens the Personas page is unaffected, and one who does can always get back by
clearing the selection.

---

## 4. API surface

Following the existing personalization routes
([internal/httpapi/personalization.go](../internal/httpapi/personalization.go)):

```
GET    /api/personas                     -> { personas: [...], active_id, options: {...} }
POST   /api/personas                     -> create; 400 invalid, 409 at limit
PATCH  /api/personas/{id}                -> partial update, same validation
DELETE /api/personas/{id}                -> 404 unknown; sessions keep their id
POST   /api/personas/{id}/duplicate      -> copy incl. portrait, name + " copy"
GET    /api/personas/{id}/portrait       -> JPEG; 404 when unset
POST   /api/personas/{id}/portrait       -> upload; 415 non-JPEG, 413 oversize
DELETE /api/personas/{id}/portrait       -> revert to the generated monogram
POST   /api/chat/sessions/{id}/persona    -> { persona_id } ; "" clears
```

`options` carries the enum lists (registers, styles, focus areas) from the same
single source that already feeds the Settings selects, so the page never
hard-codes a vocabulary the server validates.

Per-message provenance already records prompt set, provider, and model; add
`persona_id` and `persona_name` to it. That is what makes a mid-session persona
switch legible without inventing a `system` message role — the transcript can
render a divider *derived* from a provenance change between adjacent messages,
and the `messages.role` CHECK constraint stays untouched.

---

## 5. The page

### 5.1 Navigation

New route `#/personas`, second in the rail, directly under Chat — a persona is
chat furniture, closer to Chat than the content libraries are:

```
Chat · Personas · Preset modes · Pattern library · Videos · Settings
```

Add `PersonaIcon` to `shell/icons`, one `LINKS` entry in
[NavRail.tsx](../web/src/shell/NavRail.tsx), one branch in
[App.tsx](../web/src/App.tsx). `routeBase()` needs no change beyond the new
entry — it already derives valid bases from `LINKS`.

### 5.2 Tiles

A portrait grid, sibling to `.media-grid` rather than a reuse of it: video
covers are 16/9 and personas are portraits, so the aspect ratio, the text
placement (overlaid, not below), and the tile minimum all differ.

```css
.persona-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(216px, 1fr));
  gap: 12px;
}
.persona-card {              /* <button>, not a div */
  position: relative;
  aspect-ratio: 3 / 4;
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm);
  background: var(--surface-2);
  text-align: left;
  cursor: pointer;
}
```

Borrowed verbatim from `.media-card`, because they are already right and
consistency is the goal: `min-width: 0`, the `:hover`/`:focus-visible`
`border-color: var(--accent)` pair, `outline: 2px solid var(--focus)` with
`offset: 2px`, and `[aria-disabled="true"] { opacity: 0.56 }`.

Structure inside a tile:

- **Portrait** — `<img>` at `position: absolute; inset: 0; object-fit: cover`.
  The `inset: 0` is not incidental: a media thumbnail that contributed layout
  height once clipped its own badge under `overflow: hidden`, and taking the
  image out of flow is what fixed it.
- **No portrait** → a monogram plate (first grapheme of the name) on
  `var(--bg-inset)`, matching the `.nav-brand-mark` and `.startup-mark` idiom
  already in the shell. No broken-image frame, and no placeholder that looks
  like a failure.
- **Scrim + copy** — bottom-anchored gradient to `var(--surface-1)` carrying
  `<strong>` name (one line, ellipsis) and a 2-line clamped description. Text
  sits on the scrim, not on raw image, so contrast does not depend on the
  picture.
- **Chips** — top-right: register, and `active` when it is the session's
  persona. Bottom-left over the scrim: lore count when non-zero. Reuse
  `.badge`.
- **Active state** — `aria-current="true"` plus a 2px accent ring. The active
  persona must be identifiable at a glance in a grid of twelve.

### 5.3 The "New persona" tile

First child of the grid, identical footprint, `border-style: dashed`, centred
plus glyph and label. Two details that decide whether it feels right:

- It is a real `<button>` in DOM order, so keyboard users reach it first and
  screen readers announce it as a control among cards, not as a card.
- **On an empty library it is the only tile** — with the page lede doing the
  explaining. Not an empty-state panel that gets swapped for a grid at n=1;
  that transition is jarring and doubles the layout code.

### 5.4 Editor

Right-side drawer over the grid, not a route change and not a stacked modal
(exactly one layer — [ui-design.md §Settings](ui-design.md)). The grid stays
visible so the persona being edited stays in context.

Fields, in the settings idiom so it reads as one app: `h2` naming the persona,
then `.group` cards with `h3.group-title` per category — **Identity** (portrait,
name, description), **Voice and style** (register, reaction style, TTS voice),
**Behavior** (prompt set, preferred tags, default focus area), **Lore**
(section 7). Spacing is container-owned at the established 10 / 12 / 18.

Immediate-apply for toggles and selects; explicit **Save** for the text fields
(a description is not something to persist per keystroke). Delete is
confirmed and says what survives — sessions keep their transcripts.

### 5.5 Chat integration

- The Chat header shows the active persona's portrait and name beside the
  existing mood chip, and clicking it opens a compact switcher. Going to a
  separate page to change who you are talking to is a trip too many.
- New sessions inherit the last-used persona.
- Switching mid-session is allowed, takes effect on the next turn, and is
  visible via the provenance-derived divider (§4). Silent switching is the
  failure mode to avoid: a reply in a different voice with no explanation reads
  as a bug.
- `last_used_at` updates on selection, which is what orders the grid.

---

## 6. Slices

Each is independently shippable and independently revertible.

| # | Scope | Status |
| --- | --- | --- |
| 1 | Store + migration v16, validation, tests | **Shipped.** `internal/persona`; `personas` table plus `chat_sessions.persona_id` by guarded `ALTER`. |
| 2 | `personas` CRUD API + portrait upload/serve | **Shipped.** `internal/httpapi/personas.go`; reuses the thumbnail path-safety and atomic-replace shape. |
| 3 | Route, grid, new-persona tile, monogram fallback | **Shipped.** `#/personas`, second in the rail. |
| 4 | Editor drawer, duplicate, delete | **Shipped.** Browser-side canvas resize to JPEG at max edge 512. |
| 5 | Session binding + chat header switcher + provenance | **Shipped.** `PUT /api/chat/sessions/{id}/persona`; provenance carries `persona_id` and `persona_name`. |
| 6 | Reaction-style axis composition | **Shipped.** `chat.ReactionStyle`, composed between the voice identity and the contract. |
| 7 | Lore | **Not built.** Gated on the §7.2-A composition inspector and the §7.4 measurement, because the budget control it needs cannot be honest without them. `persona_lore` is deliberately not in the v16 schema: an unused column is a claim the feature exists. |

Slices 1–5 change **no prompt bytes** for anyone who does not create a persona,
and slice 6 changes none for a persona left on `neutral`. Both are asserted by
test (`TestNeutralStyleLeavesThePromptByteIdentical`), which is the property that
made this safe to ship in one pass.

### What the live pass caught

Two things that source reading did not, recorded because both generalize:

- **`<label>` is `display: inline`, so the container-owned vertical rhythm did
  not apply to it.** The measured gaps in the editor drawer were 1px where 10 or
  12 was intended and 23px where 12 was — the same "spacing declared in the wrong
  place" family as the earlier settings pass, arriving through a different door.
  The fix is the existing convention: a field label is `<label className="field">`,
  which supplies `display: flex; flex-direction: column`. After it, every gap in
  the drawer measures exactly 10 / 12 / 18.
- **A 200 response with an unexpected body shape crashed the chat route.** The
  switcher renders in the chat header, so `payload.personas` being absent threw
  during render and the error boundary took out the whole conversation. Normalized
  at the client boundary instead of in each component: a persona is decoration on
  top of chat and must never be able to break it.

Verified live against the running app: the persona axes reach the composed prompt
(`REACTION STYLE - TENDER`, the quoted name, the intimate identity block), the
settings document is untouched by a selection, a portrait round-trips
byte-for-byte with `nosniff`, a non-image is refused with 415, an unknown style
is refused with 400, and the drawer never takes Stop's click — Stop is
`z-index: 1200` against the drawer's 40, confirmed with `elementFromPoint` at
mobile width where the drawer covers the full viewport.

---

## 7. Lore and longer context

### 7.1 Why the concern is real, stated mechanically

The intuition that more lore may cost motion accuracy is correct, and it is
worth being precise about the mechanism rather than treating it as vibes:

1. **The system prompt is already long.** `composeSystem` concatenates behavior
   text, voice identity, the contract, the enabled-pattern catalog as JSON,
   live motion context, chat profile, mood state, recent assistant lines,
   memories, a final voice check, a language reminder, and the final output
   guard.
2. **Motion correctness depends on the contract surviving that.** The contract
   is code-appended and the final guard is deliberately *last*, because
   small quantized models weight recency. Lore inserted after the contract
   pushes it further from the generation point.
3. **History is capped at 12 messages** (`maxHistoryMessages`, hard-coded).
   That cap is the actual reason conversations feel short — not the model's
   window.
4. **Violations cost latency twice.** A malformed response triggers `RepairPrompt`
   and a second round-trip. On a 3B model, more instructions → more repairs →
   slower motion response, which is felt physically, not just measured.

So the honest control is a **budget with a live meter**, not a boolean labelled
"more lore". A boolean invites a user to turn it on, feel the model get worse at
motion, and conclude the app is broken.

### 7.2 Ideas, ranked by leverage-to-risk

**A. Prompt composition inspector — strong candidate, small.**
A Diagnostics view listing every section the model will receive with its
character count, and the exact composed prompt behind a copy button. This is
the diagnostics-report pattern (what is displayed is byte-for-byte what is
copied) applied to prompts. It costs almost nothing, it is the tool that makes
every other idea here tunable instead of guessed at, and it is squarely in the
app's inspectability stance. **Build this first** — before any lore feature —
because it is also how a user diagnoses "why is my model ignoring me".

**B. Configurable history depth — strong candidate, small.**
Replace hard-coded `maxHistoryMessages = 12` with a setting (6 / 12 / 24 / 40)
carrying a plain-language note that higher values cost latency and, on small
models, contract accuracy. Highest value per line of code for "longer
conversations". The 12-message default stays.

**C. Keyword-triggered lore entries — strong candidate.**
Rather than injecting a whole persona's lore every turn, store lore as *entries*
(text + keywords + enabled) and inject only entries whose keywords appear in the
recent turns. Cost scales with relevance, not library size, which is what makes
deep lore affordable at all. The `memories` table is already exactly this shape
minus a `keywords` column and an optional `persona_id` — and the reference
screenshot's `+ Memory` chip is this feature.

Ships as: `persona_lore(id, persona_id, text, keywords_json, enabled, created_at)`,
a hard cap on injected entries per turn, and the same quoted-as-data treatment
every user string already gets.

**D. Per-model context budget — worth scoping.**
The tradeoff depends on the model, and the app already has a managed model
inventory (schema v9). Store the budget per model with measured defaults: the
11.9B that passed 13/13 gets a generous budget, a 3B Q4 gets a small one. This
is what makes the toggle a *recommendation* rather than the user's guessing
game.

**E. Visible, editable session recap — worth scoping.**
When history exceeds the depth, summarize the dropped prefix into a bounded
"session so far" block. The obvious objection is that this is model-written
hidden state, which is exactly what this app rejects. The fix makes it a
feature: render the recap in the chat UI as a disclosure the user can read,
edit, or clear. Then it is inspectable state, not a hidden memory. It must be
quoted as data and stripped of anything contract-shaped.

**F. Two-pass motion/prose split — research spike, highest ceiling.**
The principled answer that removes the tradeoff instead of managing it. Pass 1:
a short, lore-free, contract-only call that decides motion. Pass 2: a
lore-rich, motion-forbidden call that writes the reply. Lore can then be
arbitrarily deep with *zero* effect on motion accuracy, because the deciding
call never sees it.

The non-obvious upside: motion can dispatch the moment pass 1 returns, before
prose is written — so motion response gets **faster** than today, where the
device waits on a full prose generation. The costs are real (two calls, two
failure modes, a larger change to `service.go`) and it needs a spike before
anyone commits. But it is the shape that makes the whole toggle unnecessary,
and it composes cleanly with the existing capability gates: pass 2 simply runs
with `Motion: false`.

**G. Per-persona memory scoping — worth scoping, decide deliberately.**
Currently memories are global. Recommendation: **keep them global** and let
lore be the per-persona channel. Global memories are what makes the app know
*the user* across every persona; lore is what makes each persona distinct.
Splitting memories per persona would fragment the former to solve a problem the
latter already solves. Revisit only if users report cross-persona bleed.

**H. Lore-aware autopilot — deliberate non-goal for now.**
Letting lore shape autonomous motion decisions is hidden escalation with extra
steps: narrative state driving the device without a visible user act. If it is
ever revisited it needs its own armed, visible mode and its own design doc.

### 7.3 How the toggle should actually appear

One control, in the persona editor's Lore group, with the meter from idea A
beside it:

- **Off** (default) — lore never enters the prompt.
- **Relevant only** — keyword-triggered entries, capped (idea C).
- **Full** — everything, up to the budget, with the measured cost stated: what
  it adds in characters and what the adherence numbers were for the selected
  model.

"With the measured cost stated" is load-bearing. The numbers have to come from
somewhere real:

### 7.4 Measurement harness

The evidence path already exists: `internal/chat/live_prompt_eval_test.go` and
`live_prompt_managed_eval_test.go` sit behind `//go:build liveeval`, and the
2026-07-20 matrix they produced (13/13 on Gemma 4 11.9B Q4_0; a Granite 4.1 3B
Q4_K_M completing every scenario with repair where needed) is the template.

Extend that into a lore-budget scorecard: for each (model, budget) pair, run the
existing turn matrix and record **first-pass valid %, repair rate, p50 latency
to motion dispatch, and speed-band violations**. That table is what the warning
text quotes and what idea D's per-model defaults are derived from. Without it,
the toggle's warning is a guess wearing a lab coat — and a guess is exactly what
"maybe this decreases motion control ability" deserves to have replaced.

---

## 8. Non-goals

- **No community browsing, importing, or downloading of personas.** The
  reference is a marketplace; this is a local app. A share-file format could
  come later under the same checksum/consent machinery as models, but it is not
  part of this page.
- **No image generation.** Portraits are files the user supplies.
- **No persona-driven limits or capabilities** (§3).
- **No hidden state.** Every string a persona contributes is visible in the
  editor and, via idea A, in the composed prompt.
- **No second personalization system.** If a field cannot be expressed as a
  value of a code-owned axis, that is a signal to add an axis, not to add a
  free-text injection point.

## 9. Relationship to other docs

- [lso-merge-alternatives.md](lso-merge-alternatives.md) §Decision 3 —
  the constraint this design implements; personas-as-presets closes that
  decision, and an LSO persona row imports as values across the axes.
- [lso-merge-integration.md](lso-merge-integration.md) — the Rockfire lineage
  has its own `personas` table, deliberately left intact for the explicit data
  import phase. That import becomes a field mapping against §2.1 rather than a
  schema negotiation.
- [feature-ideas.md](feature-ideas.md) §A — the "AI display name, profile
  picture, splash personalization" row is scoped by §2 here.
- [chat-voice.md](chat-voice.md) — the measurements that produced the axis
  shape; the reaction-style axis follows its composition pattern.
- [llm-control-surface.md](llm-control-surface.md) — the live matrix §7.4
  extends.
- [media-tooling.md](media-tooling.md) — the thumbnail store whose path-safety
  and atomic-replace shape portraits reuse, and the reason server-side image
  scaling is avoided.
- [ui-design.md](ui-design.md) — one layer, immediate-apply vs explicit-commit,
  and the settings tab shape the editor follows.
