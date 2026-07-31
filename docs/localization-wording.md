# Localization and Wording

## Status

MagicHandy ships five locales for the browser UI, installer/update flow, and
built-in chat reply prompts:

| Locale | Native name | Browser UI | Installer | Built-in chat prompt |
| --- | --- | --- | --- | --- |
| `en` | English | Yes | Yes | `magichandy_motion_v1` |
| `es` | Español | Yes | Yes | `magichandy_motion_v1_es` |
| `pt-BR` | Português (Brasil) | Yes | Yes | `magichandy_motion_v1_pt_br` |
| `zh-Hans` | 简体中文 | Yes | Yes | `magichandy_motion_v1_zh_hans` |
| `ja` | 日本語 | Yes | Yes | `magichandy_motion_v1_ja` |

English remains the default for migrated settings and unattended installs that
do not specify a locale. No right-to-left locale is currently supported.

## Sources of Truth

Do not maintain a second wording table in documentation. The shipped catalogs
and prompt code are canonical:

- Browser UI: `web/src/i18n/locales/*.json`
- Browser runtime and typed translation API: `web/src/i18n/index.tsx`
- Browser catalog/static-string audit: `web/scripts/check-localization.mjs`
- Installer/update catalogs: `scripts/installer/locales/*.json`
- Installer localization runtime: `scripts/installer/InstallerSupport.psm1`
- Built-in prompt prose: `internal/chat/prompts.go` and
  `internal/chat/prompt_localization.go`
- Prompt protocol strategy: `docs/prompt-localization-strategy.md`

`web/dist` is generated from the browser catalogs and remains the only embedded
frontend. Edit `web/src`, run the checks, and rebuild; never edit `web/dist` by
hand.

## Selection and Persistence

The browser UI language and default chat reply language are separate choices:

- `ui.locale` is a backend-owned setting. The browser renders from the backend
  snapshot and updates `html[lang]` when it changes.
- The selected built-in prompt-set ID controls the default language of model
  replies. Choosing a custom prompt set leaves language behavior to that custom
  prompt rather than adding an English override.
- Settings > General changes the browser UI language.
- Settings > Prompt changes the active built-in prompt set and therefore the
  reply language.

The source installer asks for both choices before any other decision. Changing
the UI language immediately changes all later decision-tree questions. Schema 2
of `install-state.json` stores `ui_locale` and `chat_locale`; schema 1 migrates
to English without discarding any prior choice. First installation and explicit
reconfiguration invoke MagicHandy's language-only configuration mode. Ordinary
updates preserve the current SQLite UI locale and prompt selection, including a
custom prompt; installer state controls only the update script's own language.

`update.ps1` restores the saved UI language before showing its banner or asking
a question. `change-language.ps1` always starts with the native-name language
list. It updates app settings and matching installer state only after it proves
the running process, service identity, and data profile; ambiguous process forms
or profile mismatches are refused. If a later write fails after a verified app
was stopped, the script attempts to restore that app before reporting partial
success. Low-level package-manager, compiler, download, and source-control
diagnostics may remain English so exact upstream failures stay searchable; all
choices, explanations around choices, safety confirmation, plans, summaries,
and completion text use the selected installer locale.

## Wording Rules

### Meaning and tone

- Preserve meaning, permission, severity, and whether a command is immediate or
  only descriptive.
- Use concise, work-focused UI language. Do not add promotional copy or explain
  the interface inside the interface.
- Preserve established product and protocol names: MagicHandy, The Handy,
  llama.cpp, Ollama, Faster Qwen3-TTS, Chatterbox, Parakeet, Intiface,
  Bluetooth, GGUF, WAV, JSON,
  HTTP, CUDA, CPU, WGPU, and ONNX.
- Keep units and version identifiers exact. Localized prose may use locale
  decimal punctuation, but paths, flags, IDs, hashes, and code remain verbatim.
- User-authored names, messages, memories, persona text, transcripts, model
  names, filesystem paths, and imported pattern names are data. Do not translate
  them automatically.

### Adult language

MagicHandy is an adults-only intimate-device application. Translation must not
silently sanitize the source:

- Translate explicit sexual wording at the same directness and register as the
  English source.
- Do not replace explicit partner/persona text with clinical euphemisms.
- Do not make neutral safety, settings, transport, or diagnostic text erotic.
- Preserve anatomy selection accurately. Custom anatomy text remains verbatim.

This rule applies to built-in prompt prose and UI labels. It does not authorize
translating or rewriting user-authored content.

### Safety and state

- `Stop`, connection loss, controller ownership, transport failure, and
  commanded-versus-estimated position must remain unambiguous.
- Red is reserved for Stop/error semantics and green for connected/running
  semantics regardless of locale.
- Never turn an uncertain state into a confident localized claim. For example,
  retain “commanded” or “estimated” qualifiers.
- Emergency Stop remains available through `Esc`; translated wording must not
  imply that a transport stop was confirmed when only local teardown succeeded.

## Browser Catalog Contract

English display strings are stable catalog keys. `t("...")` accepts only keys
from the English catalog at compile time, and every non-English catalog must
contain the exact same keys.

Dynamic values use named placeholders in complete grammatical templates:

```tsx
 t("Lengths differ by {duration}", { duration: formatDuration(delta) })
```

Do not construct a sentence from adjacent translated fragments. Word order,
pluralization, punctuation, and noun form differ by language. The localization
audit rejects common adjacent-fragment patterns and verifies placeholder parity.
Keep protocol values separate from translated display labels.

`translateKnown` is for a bounded set of backend-owned status strings. Unknown
backend errors remain verbatim rather than being guessed, partially translated,
or stripped of diagnostic detail.

The provider lazy-loads non-English catalogs. English is bundled with the main
chunk; changing locale updates translation context without remounting the
application, so drafts, request guards, and browser-owned connections survive.
A failed chunk load retains the prior catalog and exposes an explicit retry.

## Prompt Contract

Prompt localization is hybrid:

- Localize behavior, persona, anatomy, mood, memory headers, recent-reply
  framing, voice instructions, reply-language reminders, and repair-language
  reminders.
- Do not translate user/model-authored values. Persona, anatomy, and recent
  replies are normalized, bounded, and JSON-quoted; saved memories are bounded
  list entries.
- Keep authoritative motion-state framing in English.
- Keep JSON keys, enum values, pattern IDs, and the final machine contract in
  English.
- Keep the code-owned final output guard last.

See `docs/prompt-localization-strategy.md` for ordering, custom-prompt behavior,
and live Gemma evidence.

## Installer Catalog Contract

Installer catalogs are UTF-8 JSON because `InstallerSupport.psm1` itself stays
ASCII-safe for Windows PowerShell 5.1. The module reads catalogs explicitly as
UTF-8 and validates at import time that every locale has the English key set and
matching numbered placeholders.

All installer input must pass a catalog value or a value assembled from catalog
templates. Do not add a literal `-Question '...'` or literal decision-tree
`Read-Host` prompt. Package names, licenses, command flags, and the required
physical-confirmation token `STOPPED` remain exact.

## Validation

Run these for any UI wording or locale change:

```powershell
npm --prefix web run localization:check
npm --prefix web test
npm --prefix web run typecheck
npm --prefix web run build
powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts\test-installer.ps1
go test ./internal/config ./internal/chat ./cmd/magichandy
```

The checks enforce:

- JSON and UTF-8 integrity with no replacement characters;
- duplicate-key rejection plus exact key and placeholder parity across all five
  locales;
- compile-time browser key coverage, a static direct-string audit, and focused
  runtime tests for computed accessibility/error paths;
- no adjacent translated/dynamic sentence fragments;
- installer catalog runtime lookup and no hard-coded decision prompts;
- installer schema migration and localized update-plan behavior;
- locale-to-prompt-set mapping and durable SQLite persistence;
- localized prompt composition, final-guard ordering, and repair behavior.

A release-facing language change also needs rendered desktop and narrow-viewport
inspection in at least one Latin locale and one CJK locale. Check clipping,
wrapping, button labels, select options, dialog titles, status text, and the
settings save/reload path.

## Adding a Locale

1. Add the locale constant and prompt-set mapping in `internal/config`.
2. Add a built-in prompt set and localized code-owned prompt prose.
3. Add matching browser and installer JSON catalogs using a BCP 47 locale tag.
4. Add the native display name to both selectors.
5. Extend locale normalization, lazy imports, public settings options, installer
   validation, state validation, and recovery-script `ValidateSet` lists.
6. Extend unit tests, catalog audits, and the live prompt matrix.
7. Render the full application at desktop and narrow widths and review with a
   fluent speaker. Automated parity proves completeness, not natural phrasing.

Do not ship a locale with partial UI coverage or rely on English fallback to
hide missing catalog entries.
