# Settings Compaction — Provider-Scoped Disclosure

## Status

Implemented 2026-07-09 as Slice 13.5 in
[IMPLEMENTATION_PLAN.md](../IMPLEMENTATION_PLAN.md). It changes the Settings
**presentation and voice settings schema** without changing the worker
protocol or safety rules in [ui-design.md](ui-design.md).

## Problem

Settings sections have grown by accretion, worst on the Voice tab: after
Phase 13.4 it stacks ten flat controls — a master toggle, two path fields,
two argument textareas, the speak-replies toggle, the ElevenLabs key field
and its clear toggle, a status paragraph, and the worker status rows — and
every field is visible regardless of which provider the user actually uses.
A Parakeet user stares at ElevenLabs key fields; an ElevenLabs user stares
at `-server-model` placeholders. The Model tab has the same disease in a
milder form: llama.cpp *and* Ollama fields render no matter which provider
is selected.

## Principle: selection-scoped disclosure

One rule, applied everywhere: **a field is visible only when the current
selection makes it meaningful.** A provider/mode dropdown is the selector;
the fields under it are scoped to it. Corollaries:

- Switching a dropdown never destroys hidden values — unselected providers
  keep their configuration and reappear intact when reselected. Only the
  active selection is composed into runtime behavior.
- Status readouts are never hidden by disclosure: worker status rows,
  connection state, and error text stay visible regardless of selection.
- Safety rules are unaffected: nothing here touches Stop, quick controls'
  immediate-apply, or the status bar.
- Soft budget: a section that renders more than ~7 controls for a single
  selection should be split or given an Advanced disclosure before it grows
  further.

## Voice tab redesign

The Voice tab becomes **two sections on the same page** (same
`#/settings/voice` route, one Save), in this order:

```
Voice                                  (page title + master enable toggle)

── Speech input (ASR) ──────────────────────────────
Provider: [ None ▾ | Parakeet (managed, local) | OpenAI-compatible server | Custom worker ]
<provider-scoped fields>
<worker status row: dot+text, Start/Stop/Restart/Load/Test controls>

── Speech output (TTS) ─────────────────────────────
Provider: [ None ▾ | ElevenLabs (cloud) | NeuTTS Air (local) | Custom worker ]
<provider-scoped fields>
[x] Speak chat replies            (only rendered when a TTS provider is set)
<worker status row>
```

The existing `VoiceWorkers` status component splits per role and docks into
its section, so status lives beside the configuration it describes instead
of in a combined block at the bottom.

### Provider-scoped fields

**Speech input (ASR)**

| Provider | Visible fields |
| --- | --- |
| None | none (section shows only the dropdown) |
| Parakeet (managed, local) | `parakeet-server` path · GGUF model path · port (Advanced, default 8990) — both paths prefilled when the installer provisioned them |
| OpenAI-compatible server | base URL |
| Custom worker | worker path · worker args (one per line) |

**Speech output (TTS)**

| Provider | Visible fields |
| --- | --- |
| None | none |
| ElevenLabs (cloud) | API key (write-only, set-badge, clear toggle — existing secret handling unchanged) · voice ID (default Rachel) · model ID (default `eleven_multilingual_v2`) |
| NeuTTS Air (local) | `stream_pcm` runner path · reference WAV provenance · pre-encoded `.npy` codes · reference transcript |
| Custom worker | worker path · worker args (one per line) |

"Custom worker" is the escape hatch that keeps the general ADR 0003 worker
contract fully reachable — anything speaking the protocol still works with
raw path + args, including the stub worker for development.

## Backend: a provider model, not frontend sugar

The dropdown must not be a frontend template that pastes argument strings —
that would put provider knowledge in the UI, break round-tripping (parsing
args back into fields), and violate backend-authoritative state. Instead:

1. `config.VoiceSettings` gains per-role discriminators and provider fields
   (additive, settings version stays 1; absent fields default):

   ```
   tts_provider:  none | elevenlabs | neutts_air | custom
   asr_provider:  none | parakeet_managed | openai_compatible | custom
   elevenlabs_voice_id, elevenlabs_model_id
   parakeet_server_path, parakeet_model_path, parakeet_port
   asr_base_url
   ```

   The existing raw `*_worker_path`/`*_worker_args` fields remain and are
   what the `custom` provider edits. The ElevenLabs key keeps its existing
   write-only secret handling.
2. `httpapi.voiceManagerConfig` composes the actual worker command and
   arguments from the selected provider — the argument templates live in Go
   where they are tested, next to the workers they launch. `enabled` +
   `provider: none` maps to a disabled worker exactly as today.
3. **Worker binary resolution** removes path fields from the common case.
   For the known providers (`elevenlabs`, `parakeet_managed`), the worker
   executable is resolved in order: explicit override (Advanced field) →
   alongside the `magichandy` executable → the data-dir tools folder the
   installer provisions. Phase 16 packaging ships the worker binaries next
   to the app, which makes the zero-path-fields case the default; the
   resolution order is reported in worker status so a wrong pick is
   visible, never silent.
4. **Migration**: existing settings with populated worker paths load as
   `provider: custom` with all values intact — upgrades change presentation
   only, never behavior. Covered by settings round-trip tests.

## Same rule applied to the other tabs (small, same slice)

- **Model**: managed `llama_cpp` shows app-owned runtime status/build controls
  and the managed model inventory, never runner/GGUF path fields. External
  `llama_cpp` shows its base URL plus server-reported models; `ollama` shows its
  URL plus daemon-reported models. Provider and mode disclosure stays explicit.
- **Device**: the developer application ID field renders only when the ID
  source is `developer_override` (the Bluetooth bridge panel is already
  conditional).
- **Prompts & memory / Diagnostics**: already compact; no change.

## Test expectations

- Settings round-trip: every provider's fields survive save/reload; hidden
  (unselected) provider values survive a save made while another provider
  is active.
- Migration: legacy path/args settings load as `custom` with identical
  worker launch behavior (compare composed WorkerConfig).
- Composition: per-provider WorkerConfig composition unit-tested in Go,
  including binary resolution order and the port/base-URL variants.
- UI (vitest): switching providers swaps visible fields without losing
  state; speak-replies renders only with a TTS provider; worker status rows
  visible for every provider including None; secret field behavior
  unchanged (set badge, write-only, clear).
- Embed test pins the new section headings ("Speech input", "Speech
  output").

## Out of scope

- Additional voice providers beyond the four documented selections.
- Any change to worker protocol, lifecycle, the audio lease, or lockstep
  delivery ordering.
- Immediate-apply for worker config (worker configuration stays on Save;
  immediacy remains reserved for live device controls per ui-design.md).

## Relationship

- [ui-design.md](ui-design.md) — Settings rules (single layer, immediate
  quick controls, confirmed destructive actions) all still apply.
- [ui-navigation-redesign.md](ui-navigation-redesign.md) — the shell this
  page lives in (implemented).
- [voice-worker-protocol.md](voice-worker-protocol.md), ADR 0003/0007 — the
  contracts the provider model composes onto.
- `docs/legacy-parity-sweep-2026-07.md` §E — the sweep's UI backlog; the
  speak-replies quick toggle in the Chat control column is separate from
  (and compatible with) this redesign.
