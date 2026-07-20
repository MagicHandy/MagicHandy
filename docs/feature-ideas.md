# Feature Ideas Catalog — 2026-07-19 Deep Review

A deeper parity and ideas pass, from three sources:

1. **STGPT-RV itself, re-read at the settings/UI level** — the full 96-key
   `my_settings.json` surface, `index.html` controls, and `KNOWN_PROBLEMS.md`,
   which the earlier sweeps summarized but did not fully enumerate.
2. **The Codex conversation history (2026-04 → 2026-07)** — ~2,200 user
   messages across ~30 StrokeGPT/MagicHandy sessions, distilled to the wants
   and ideas that were voiced but never became tracked items.
3. **Fresh brainstorm** — features in *neither* app.

This is a catalog for discussion, not a plan; nothing here is scheduled until
it becomes a scoped slice. It extends
[parity-with-stgpt-rv.md](parity-with-stgpt-rv.md) (capability matrix) and
[llm-control-surface.md](llm-control-surface.md) (LLM-control ideas A–G and
paradigms 1–5), and it references those instead of restating them.

Dispositions: **Strong candidate** (clear value, bounded scope) ·
**Worth scoping** (value likely, design first) · **Research spike**
(unproven; time-boxed investigation only) · **Covered** (already exists) ·
**Deliberate non-goal** (recorded so it is not re-proposed) · **Rejected**
(conflicts with design, with reason).

---

## A. Deeper parity findings — STGPT-RV features not previously itemized

The sweeps dispositioned STGPT-RV's *lessons*; this table itemizes concrete
*features* visible in its settings/UI that MagicHandy's docs had not put on a
row of their own.

| Feature (STGPT-RV evidence) | Disposition for MagicHandy |
| --- | --- |
| **"I'm close" signal button** (`I'm Close` in the UI; extends milk, holds/stops edge; history 2026-04-22 asked for LLM-decided hold durations and edge counting) | **Strong candidate.** The single highest-value session signal. Fits [llm-control-surface.md](llm-control-surface.md) paradigm 5 (explicit feedback loop): deterministic immediate act (hold/ease per active mode) plus a structured signal the model reads. An edge counter is visible state, not hidden escalation. |
| **Autospeak** (`autospeak_enabled/min_seconds/max_seconds/motion_autonomy`) — the model speaks unprompted at a bounded random cadence, with a separate motion-autonomy level | **Partial in Chat Autopilot; cadence controls remain worth scoping.** PR #101 routes successful autonomous lines through the canonical Chat/TTS path and refuses to deepen an existing TTS backlog. It does not yet expose a user cadence window or autonomy level. Keep the STGPT-RV shape for that follow-up: bounded non-zero cadence, visible autonomy level, and a hard-off switch. |
| **Mood display** (`mood-display` chip) — model-reported mood as a top-bar chip | **Worth scoping.** The read-only face of idea D (visible style/mood state). Only if it is *state the model actually set*, never inferred sentiment (see non-goal below). |
| **Live motion sequence log** (timecoded scrolling log of motion events in the chat UI; repeatedly refined in history 2026-04-22) | **Strong candidate.** MagicHandy has trace export but no user-facing live log. This is the concrete form of the "trace-visible what changed" requirement — a compact, session-clock-timecoded strip of model/mode motion changes. |
| **Per-message model provenance** (hover on a reply's avatar shows model name/run details; history 2026-05-17) | **Strong candidate, small.** Chat already knows the provider/model per turn; surface it on hover/long-press. Useful when comparing models. |
| **AI display name, profile picture, splash personalization** (`ai_name`, `profile_picture_b64`, splash) | **Worth scoping (cosmetic tier).** Persona text is covered by prompt sets; the *identity cosmetics* are not. Cheap goodwill; images stay local. |
| **User anatomy selector** (`user_genitalia`, `_custom`) feeding prompt context (history 2026-05-15: model used wrong-anatomy language) | **Worth scoping.** Already half-recorded in the Phase 15 personalization notes (identity/interest selector). It is a *prompt-correctness* feature, not just personalization; local, editable, resettable like all personal data. |
| **Per-capability LLM permission gates** (`allow_llm_edge_in_chat`, `allow_llm_mode_actions_in_chat`, per-mode hands-free gates …) | **Covered by a better shape.** MagicHandy's planned control-style selector (paradigm 2) generalizes this pile of checkboxes into Manual/Assist/Director; keep voice-initiated mode changes behind their own gate (the one STGPT-RV lesson to preserve verbatim). |
| **Per-mode session time bounds** (`auto/edging/milking_min/max_time`) | **Covered/superseded** by the arrangement contract's segment bounds and the session-goal idea (paradigm 4). |
| **Program section clipping + bookmarks** (Settings tour: "Program section clipping"; funscript timeline crop with device preview, history 2026-04-23) | **Partial → clip-on-import shipped (2026-07-19).** The library's Import tab renders a zoomable/pannable funscript source timeline with action-snapped dual-thumb trim, an exact selection-length readout, and a pattern/program choice; zoom never changes submitted content and the trimmed selection goes through the normal validated import. Play-this-selection device preview and named section bookmarks remain unbuilt. |
| **TTS style tuning knobs** (`local_tts_exaggeration/cfg_weight/temperature/top_p/min_p/repetition_penalty`) | **Deliberate non-goal for now.** This is the "voice tab grew 12 knobs before defaults were proven" anti-lesson the plan already cites. NeuTTS exposes a deterministic seed instead; add expressive knobs only with listening evidence (R17 discipline). |
| **Curated starter voice pack** (STGPT-RV shipped Chatterbox sample voices; history 2026-04-21 asked for license-clean sources) | **Worth scoping.** NeuTTS requires a reference WAV+transcript, which is a cold-start wall. One or two clearly-licensed reference voices as an explicit, checksummed optional download would fix first-run TTS. |
| **ASR model size choice** (faster-whisper presets tiny→distil-large; "larger Parakeet" request 2026-04-23) | **Worth scoping, later.** Managed Parakeet is single-model today; a second, larger checksummed option is cheap once curated downloads exist (Phase 16 machinery). |
| **Firmware v3 selector / HAMP-HDSP legacy backends** | **Rejected** (ADR 0006) — recorded here so the settings-surface diff does not re-raise them. |
| **LAN/HTTPS mobile use** (whole doc + certs + mobile fixes) | **Deferred — R18** (already in the matrix; the history's mobile-Chrome-HTTPS failures are evidence for that decision doc). |
| **Internet remote + multi-user sessions** (ROADMAP #17) | **Long-horizon, already recorded** in the plan's post-parity backlog. |

## B. Ideas voiced in the Codex history that never became tracked items

Mined from the conversation digest; dates are the session dates. Items already
covered above or in the LLM-control doc are cross-referenced, not repeated.

- **Pattern blending / alternating sequences** (2026-04-21: "program it to
  alternate between multiple patterns in a sequence, blending slightly to
  avoid stutter") → the **queue paradigm** (llm-control idea 1) is exactly
  this; the history confirms user demand predates the design.
- **Ramp toggles with a countdown the model ingests** (2026-05-17: "toggles
  that encourage the LLM to ramp up or down intensity over time (with a
  counting down or up flag)") → the **session-goal/director** idea
  (paradigm 4); the countdown-as-model-input detail is worth keeping — the
  model *sees* remaining time as data instead of guessing.
- **Quick thumbs up/down on the current motion from the chat strip**
  (2026-04-21: circular like/dislike beside the speed/depth indicator, wired
  to pattern weights with opt-in auto-disable) → **Strong candidate**; the
  Phase 14 feedback contract exists, only the in-chat quick surface is
  missing. Merges naturally with paradigm 5's buttons.
- **Longer/more complex patterns as a jerkiness workaround** (2026-05-16:
  "maybe full funscripts… if primitive local models cannot handle the
  orchestration") → validates program-selection (idea B) and the curated
  catalog note that content depth gates LLM curation quality.
- **Model catalog defined in two places** (2026-04-23 maintenance note) →
  lesson, not feature: when curated downloads land (Phase 16), the catalog
  must be single-sourced (app-served manifest the installer consumes).
- **Voice reads messages the UI never showed / overlap at catch-up**
  (2026-05-17) → already answered structurally by ADR 0003 lockstep +
  single-owner audio lease; recorded as regression-test material, not a
  feature.
- **Model-comparison ergonomics** (2026-05-17 hover-provenance request;
  repeated model switching during debugging) → provenance row in §A plus a
  possible tiny "recently used models" list in Settings > Model. Low value
  until multiple runtimes are common; note only.
- **Branding/imagery direction** (2026-07-07: "Handy-like imagery… simple but
  invoking touch or movement") → recorded as the art direction for the
  [setup-wizard-design.md](setup-wizard-design.md) branding slots and any
  future logo work.

## C. Net-new ideas — in neither app

Grouped; each with an honest disposition. Safety frame for all of them: no
hidden escalation, every autonomous behavior visible and boundable, Stop
independent of everything.

### Session & routine

- **Warm-up / wind-down envelopes.** One-tap gentle ramp-in at session start
  and ease-out at end, as visible bounded envelopes over the active target
  (never raising caps). **Worth scoping** — pairs naturally with the
  session-goal director; the wind-down half doubles as a "sleep timer".
- **Local session journal (opt-in).** Duration, modes/styles used, stops,
  feedback signals — local-only, off by default, one-click purge, excluded
  from any export by default. **Worth scoping**, privacy-first; useful for
  "what did I like" and for preference training transparency.
- **Scheduled/timed sessions.** **Deliberate non-goal**: an unattended
  device-motion timer is an unattended-operation hazard and serves no
  attended use case the goal director doesn't.

### Input & control

- **Global quick controls (hotkeys/media keys/gamepad).** Speed nudge, hold,
  Stop from outside the browser tab. **Research spike**: browsers cannot grab
  global keys; this is only honest as part of a future desktop shell
  (WebView2 decision, Phase 16) — and Stop's guarantee must never *depend* on
  it.
- **Authoring by demonstration ("record a jog").** Record a manually scrubbed
  on-screen slider (or live-limit wiggles) for N seconds into an editable
  pattern draft. **Worth scoping** — the authoring pipeline (knots,
  simplification, preview) already exists; this is just a new input source
  for it.
- **A/B audition in Training.** Play two candidate patterns back-to-back on
  the device and record which won as normal feedback. **Worth scoping,
  small** — pure UI over existing playback + feedback.

### Content

- **Beat/music-derived patterns.** Import an audio file, extract an onset/
  energy envelope, propose a pattern draft into the authoring editor (never
  straight to playback). **Research spike** — real user appeal, real DSP
  scope; funscript-world references exist (FunGen et al.).
- **Community share-file index.** A curated, licensed catalog of pattern
  share-files the app can browse/download with the same checksum/consent
  machinery as models. **Worth scoping *after* curated model downloads** —
  same plumbing, new content type; moderation/licensing burden is the real
  cost. Directly answers "the LLM keeps picking the same two scripts".
- **Video + funscript sync player.** **Promoted to planned (2026-07-19)** —
  the earlier non-goal disposition was reversed by explicit direction. The
  concerns behind it survive as guardrails (no transcoding, no media
  management, no new motion pathway); the full design — library locations +
  scan, video grid/search under the library page, exact-basename funscript
  pairing, synced playback anchored to the video clock, and the hideable
  intensity-colored OSD strip — lives in
  [video-playback.md](video-playback.md).

### Platform & data

- **One-click backup/restore.** Export the data directory (settings,
  memories, patterns, programs — models excluded by size, secrets included
  only in an explicitly-marked private archive) and restore it on a new
  machine. **Strong candidate** — cheap, and it is the honest substitute for
  much of what the undecided STGPT-RV importer would do for *MagicHandy's own*
  future migrations.
- **Data-directory usage dashboard.** Models and voice trees are multi-GB;
  show per-store disk usage in Settings > Diagnostics with guarded cleanup
  actions. **Strong candidate, small.**
- **Multiple local profiles.** Separate data dirs behind a picker (distinct
  personas/limits/memories per person), preserving the single-operator
  runtime assumption. **Worth scoping**; mostly launcher/data-dir plumbing
  that already exists via `-data-dir`.
- **Privacy lock + discretion mode.** Optional PIN on launch, a panic
  hide-window hotkey (desktop-shell dependent), and a "discreet session"
  toggle that suspends journal/history writes. **Worth scoping** — aligned
  with the app's privacy-first stance; PIN is deterrence, not encryption, and
  must say so.
- **Completion notifications.** Windows toast (or tab-title badge) when a
  long operation finishes — model download, source build, import. **Strong
  candidate, small**; the tab-title form needs no new permissions.
- **Localization.** Already owned by the LSO merge decision 6 (one pipeline);
  cross-referenced here so it is not re-invented as a fresh idea.
- **Accessibility pass to "voice-only operable".** Push-to-talk exists;
  formalize an audit that every core flow (connect, start, adjust, stop) is
  completable by voice + screen reader. **Worth scoping** as an audit slice
  with fix-list, not a rewrite.

### Device & sensing

- **Multi-device dispatch.** Two linear actuators (e.g. via Intiface) driven
  from one engine. **Research spike** — the engine is deliberately
  one-owner/one-device; mirroring is easy but pointless, independent control
  is a real architecture change. Needs a use case before any design.
- **Biometric-adaptive intensity (BLE heart-rate).** **Deliberate non-goal**
  in autonomous form — it is the hardware version of sentiment-paced drift
  (hidden physiological state driving escalation). At most a **research
  spike** for *displaying* HR alongside the session journal, with any control
  coupling requiring its own explicit, visible, user-armed mode and a design
  doc.

---

## What this changes elsewhere

- [parity-with-stgpt-rv.md](parity-with-stgpt-rv.md) stays the capability
  matrix; §A above supplies candidate new rows ("I'm close", autospeak, mood
  display, motion log, provenance, clipping/bookmarks, starter voices) the
  next matrix refresh should absorb.
- [llm-control-surface.md](llm-control-surface.md) gains historical demand
  evidence (§B) for paradigms 1, 4, and 5 — no edits required there.
- Nothing in this catalog is scheduled. The plan's priority question
  (voice-quality depth vs parity milestones) is unchanged and remains the
  maintainers' call.
