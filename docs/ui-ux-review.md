# UI/UX Review — 2026-07-10 (post-Phase-13 live pass)

Reviewed against `main` at `53555ecd` (PR #49 merged: provider-scoped voice
Settings, NeuTTS adapter, push-to-talk, Chat speak-replies control) by driving
the built binary headless and inspecting the **live rendered DOM** at
1680×940, 1038×1018, and 375×812. Findings carry file/line references against
that commit.

Status: H1, M1–M6, and L1–L3 were fixed by Slice 13.8 (PR #51), which also
replaced the sidebar's active-link accent bar with a soft azure fill. L4
(recording level feedback) stays deferred to the hands-free capture slice.

A previous review dated 2026-07-08 lived at this path (never committed). All
of its findings (C1–C3, H1–H4, M1–M9, L1–L6) were consumed by the
`claude/react-frontend` fix commit and later merges; this pass spot-checked
the load-bearing ones live: `--go`/`--radius-md` no longer referenced, active
nav link rounded (8px, inset accent), toast centered over the workspace
column (`left: calc(50% + var(--rail-width) / 2)`), workspace content capped
and centered on wide screens (split 1180px in a 1444px workspace at 1680px
viewport), chat fills the desktop column (log 591px tall), user avatar "Y",
save row always rendered. That review is superseded by this document.

## Verified working (live)

- Settings → Voice matches the Slice 13.5 design: one page, Speech input
  (ASR) and Speech output (TTS) sections, provider dropdowns
  (none/Parakeet-managed/OpenAI-compatible/custom and
  none/ElevenLabs/NeuTTS/custom), selection-scoped fields for every variant,
  collapsed Advanced override, write-only ElevenLabs key with set-badge.
- Legacy migration: a v1 settings file holding only a raw TTS worker command
  loaded as `tts_provider: "custom"` with the command intact; absent ASR
  became `none`. Verified against real pre-#49 scratch data.
- Worker lifecycle from the UI: Start spawned the configured worker,
  handshake surfaced `provider v0.1.0 · protocol v1 · model unloaded ·
  queue 0`, and the action row stayed contextual (Stop/Load model while
  running; Send test only once loaded).
- Full TTS round trip: load model → test request → `done`, audio served with
  `Content-Type: audio/wav` under the controller lease.
- Chat page: mic hold-to-talk button and speak-replies quick toggle are
  present (the toggle persists through `PUT /api/voice/preferences` and
  round-trips into settings); chat history seeds from the canonical server
  log; deterministic stop exchange rendered from persisted state.
- Modes/Library placeholder states render as designed (Autopilot coming-soon,
  Library Phase 14 empty state).

## High

### H1. Stacked (≤900px) chat layout paints the log over the composer

At the single-column breakpoint the chat panel no longer gets its height from
the stretched split row (`components.css:504-531`), so it collapses to
content height. Measured at 375×812: `.chat` resolves to 214px,
`.chat-log-shell` to 74px, but `.chat-log` keeps `min-height: 240px`
(`components.css:364-372`) and overflows the shell (`overflow: visible`,
`components.css:382-387`), painting over the form: the textarea (y=270) sits
fully behind the log (y=181–421), the mic button is hidden, and a lone
disabled Send shows through mid-panel. The composer is unusable on mobile
and any stacked width.

**Fix:** at `max-width: 900px`, give the log a bounded height and release the
min-height so the composer keeps its space, e.g. `.chat-log { min-height: 0;
height: clamp(240px, 45dvh, 480px); }` (or give `.chat` an explicit
`min-height` from the viewport). Add a vitest/live check that the textarea's
box does not intersect the log's at 375px.

## Medium

### M1. Mic button is not gated by ASR configuration or worker state

`ChatPanel.tsx:352-363` renders "Hold to talk" unconditionally. With
`asr_provider: "none"` (the default), or voice globally disabled, or the
worker configured-but-stopped, the user can record 30 seconds and only then
get a failure toast; nothing explains that speech input is not set up.
Recording is also allowed when the ASR worker is stopped, though workers are
deliberately never autostarted, so the submit can never succeed.

**Fix:** hide the button when no ASR provider is configured (mirroring
`VoiceQuickControls`); when configured but not running, disable it with a
hint ("Start the speech-input worker in Settings → Voice"). Keep the
never-autostart invariant.

### M2. Speak-replies quick toggle ignores `voice.enabled`

`VoiceQuickControls.tsx:8-9` checks only `tts_provider !== "none"`. With a
provider selected but "Enable voice workers" off, the toggle appears and
saves, yet `enqueueSpeech` (`internal/httpapi/chat.go:283-298`) drops every
request because it checks `Voice.Enabled`. The user gets a working-looking
toggle that does nothing.

**Fix:** gate on `voice.enabled && tts_provider !== "none"` (same gate for
the mic button per M1).

### M3. Voice request outcomes are invisible; test audio never plays

`VoiceWorkers.tsx:88` lists only `queued`/`active` requests, so a fast
failure (verified live: speak with model unloaded → `failed:
model_not_loaded`) or a fast success leaves no visible trace. "Send test"
(`VoiceWorkers.tsx:125`) reports nothing on completion and the produced clip
is never played, so a TTS test cannot actually be heard.

**Fix:** keep a per-role "last result" line (state + error code + duration)
from the request log, and play the test clip through the existing
lease-gated audio endpoint on completion.

### M4. Speaking requires a manual "Load model" click nobody will find

ElevenLabs and NeuTTS workers both start with the model unloaded
(`elevenlabsworker.go:388-395`, `neuttsworker.go:296-300`) and
`enqueueSpeech` never sends a load, so the visible end-to-end path is:
Settings → Start → Load model → back to Chat → toggle Speak replies. Skip
"Load model" and every reply silently fails with `model_not_loaded`
(verified live). For ElevenLabs the load is a pure formality (cloud API, no
local model), making the extra click pure friction.

**Fix:** after a user-initiated Start of a first-party provider, send `load`
automatically (this does not violate never-autostart — the user started the
process), or auto-load on first speak. At minimum surface "model unloaded —
replies won't be spoken" next to the speak-replies toggle.

### M5. No voice health signal outside Settings → Voice

`StatusBar.tsx` renders motion/core/controller/timer only. A crashed TTS
worker, or speak-replies enabled with the worker stopped/unloaded, is
invisible from the chat page: replies just stop speaking. ADR 0003 makes
crashes visible in the settings rows, but nothing propagates to where the
user actually is.

**Fix:** selection-scoped status readout — add a voice dot to the status bar
only when voice is enabled and unhealthy (crashed, or speak-replies on with
the TTS worker not running/loaded). No readout when voice is off, keeping
the bar quiet for non-voice users.

### M6. Worker Start acts on the saved provider, not the selected one

The worker rows (`VoiceSettingsPanel.tsx:57,81`) operate on the persisted
config. Switch the provider dropdown (unsaved) and click Start: the previous
provider's worker starts, with the status row sitting under the new
provider's fields.

**Fix:** disable the worker action row while the voice section has unsaved
changes, with a "Save to apply" hint.

## Low

### L1. Controller readout collapses to an unlabeled dot ≤1100px

`shell.css:233-237` hides `.status-readout-controller .status-text`, leaving
a bare dot after "core ok" that reads as a stray artifact; its green/amber
meaning (controller vs read-only) is not discoverable, and the dot alone is
not announced usefully by screen readers.

**Fix:** keep a short text ("you" / "read-only") or add
`aria-label`/`title` to the readout when the text is hidden.

### L2. Worker status row label repeats the section heading

Under "SPEECH INPUT (ASR)" the row is again titled "Speech input (ASR)"
(`VoiceWorkers.tsx:36`). Inside the per-role sections the row could simply
say "Worker".

### L3. The 30-second recording cap is silent

`ChatPanel.tsx:260` stops the recorder at 30s with no prior indication.
Show the cap (tooltip on the mic button) or a countdown while "Listening".

### L4. No recording level feedback

While recording, the only signal is the "Listening" label. A minimal level
indicator (or pulsing dot consistent with the design system) would confirm
the microphone is actually capturing. Roadmap-adjacent; fine to defer to the
hands-free slice.

## Suggested fix order

1. **H1** — the composer is unusable on stacked widths today.
2. **M1 + M2** — one shared "voice role is usable" gate for the chat
   controls; small, high-frequency friction.
3. **M4** — auto-load after user Start; unblocks the advertised
   speak-replies flow.
4. **M3 + M5** — feedback loop (last result, test playback, status-bar
   health).
5. **M6, L1–L3** — polish.

Suggested owner note: H1/M1/M2/L1 are frontend-only; M3–M5 touch
`internal/httpapi` + frontend; M6 is frontend state tracking.
