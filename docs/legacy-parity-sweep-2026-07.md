# Legacy Parity Sweep — 2026-07-09

A re-read of the StrokeGPT-ReVibed working notes, `Changelog.txt` (through PR
#333), `KNOWN_PROBLEMS.md`, and `ROADMAP.md`, cross-checked against what
MagicHandy has actually shipped through Phase 13.4. Purpose: catch learnings
and parity items that postdate the original plan mining so they cannot
silently vanish (risk R2). Each item records where it now lives; nothing in
this file is a commitment by itself.

Dispositions: **Adopted** (tracked in a phase/doc), **Covered** (MagicHandy
already does it — evidence noted), **Deferred** (recorded, deliberately not
scheduled), **Rejected** (conflicts with MagicHandy design, with reason).

## A. Motion quality — hardware-validated lessons (June 2026 PRs)

These came from real-device iteration in StrokeGPT-ReVibed after MagicHandy's
motion phases were planned. They are requirements on Phase 14's pattern
catalog and the mode planners, not retrofits to the shipped engine.

1. **Monotone-cubic (PCHIP) time-parameterized sampling** (PR #319). The
   Catmull-Rom/index-parameterized sampler overshot; monotone cubic in wall
   time gave C1 continuity, zero-velocity reversals, and cut max wall-clock
   acceleration ~8900 → ~2500 pos/s². *Covered in Phase 14*: the shared
   content curve is wall-time PCHIP; tests assert continuity, no overshoot,
   and zero velocity at reversal knots.
2. **Parametric pattern catalog with enforced budgets** (PR #322). 38
   patterns generated parametrically with wall-clock acceleration and
   reversal-gap budgets enforced by the generator — not hand-keyframed.
   *Covered in Phase 14*: built-ins are generated from parameters and every
   generated definition is checked against acceleration and reversal-gap
   budgets. Expanding beyond the small baseline catalog remains content work,
   not a second playback architecture.
3. **Relative-span normalization, projected exactly once** (PR #323).
   Patterns authored 0–100 relative and projected into the stroke window at
   dispatch; band-authored patterns projected twice degrade into
   barely-moving twitches. *Covered in Phase 14*: patterns remain semantic
   0–100 through planning and are projected only by the transport boundary;
   the engine regression test checks a non-default stroke window.
4. **Routine cycle floor** (PR #324). On-device: pattern cycles under ~6 s
   stuttered; ~6.6 s+ felt smooth (burst shapes exempt). Time-only stretch,
   amplitude unchanged. Not reproducible in synthetic wire analysis — a
   hardware-only signal. *Implemented, hardware confirmation pending*: routine
   curves use a 6600 ms time-only floor and bursts a 500 ms floor. The physical
   feel check remains open because synthetic evidence cannot prove the source
   threshold.
5. **Latency-aware mode dwell floor** (PR #331). Continuous scripted modes
   hold an applied target at least the recent measured command latency plus
   padding (capped 12 s) so Cloud REST latency spikes don't make planners
   replace streams faster than the device settles. *Covered in Phase 14*: mode
   dwell is at least recent measured command latency plus 750 ms, capped at
   12 s; the floor and cap have regression coverage.
6. **Live-stream adjustment vs. replacement for plain chat tweaks**
   (PR #320). Plain chat speed/depth adjustments should modify the live
   stream, never flush-and-replace (stop/go "morphs"). *Covered*: the
   MagicHandy engine retargets in-stream (Phase 7, one continuous stream
   proven across many retargets), and quick settings refresh active motion
   (ADR 0002 Invariant 9).
7. **Non-action chat must never invent a motion change** (PR #325). An
   affirmation ("that feels good") once fell into a fallback that retargeted
   active motion. *Covered*: `action: none` dispatches no target; target-field
   inheritance preserves active content, including a finite program during a
   speed-only chat retarget. API regressions cover both no-action and active-
   program cases.
8. **Pooled HTTP session for device commands** (PR #321). Per-command
   TCP+TLS setup compounded into 1–2 s command latency in Python.
   *Covered*: the Go cloud transport uses one shared `http.Client`
   (keep-alive pooling is Go's default transport behavior).
9. **Inertia at reversals and stops; no visible steps** (notes). The notes
   ask for inertia-slowing on direction changes and interpolation without
   perceptible steps. Largely subsumed by items 1–4; keep the phrasing in
   Phase 14's feel checklist so the on-device evaluation asks this question
   explicitly.

## B. Chat and voice behavior

10. **Spoken text must match displayed text** (notes: "text doesn't always
    show up in chat but the voice model gets it"). *Covered*: Phase 13.0
    lockstep emit/enqueue with byte-identical test.
11. **TTS cut off at the end of replies** (notes). *Adopted*: added to the
    Phase 13 provider validation checklist — verify final-sentence
    completion on real providers (sentence streaming should prevent it; it
    must be checked, not assumed).
12. **Modes narrate naturally** (notes: the model "blares out motion
    sequences" in preset modes). *Adopted*: Chat Autopilot rule — any
    model-generated speech goes through the same reply text that chat
    displays, phrased as natural language; never raw sequence identifiers.
13. **Voice on/off toggle in the top bar** (notes). *Adopted, amended*: the
    MagicHandy status bar is status-only by design (no controls), so the
    speak-replies quick toggle belongs in the Chat control column next to
    the other immediate-apply controls. **Covered**: the Chat control column
    now has an immediate speak-replies toggle; the status bar remains
    status-only.
14. **Stop guards vs. deliberate LLM stops** (notes). Mode-start decisions
    were once guarded against local-model stop actions. *Covered*:
    stop-intent is deterministic (chat stop fast-path bypasses the LLM);
    chat stops end modes via `NotifyChatStop`/`NotifyUserStop`; Autopilot
    inherits the same rules.

## C. Pattern library and training (Phase 14 inputs)

15. **Training module** (notes; STGPT "motion training"): device plays
    patterns (including generated ones), user rates them; ratings become
    visible, editable weights. **Feedback must never disable anything or
    change weights without the change being visible and reversible in the
    GUI** (notes, verbatim requirement). *Covered in Phase 14*: Training shows
    the before/after weight, each rating has an exact undo ledger, direct edits
    cannot be overwritten by stale undo, and auto-disable is an explicit
    opt-in.
16. **Patterns as individual shareable files + import** (notes), including
    **funscript import** (notes; STGPT roadmap). Ignore long inactivity gaps
    when importing funscripts (they were video-matched). *Covered in Phase 14*:
    `.mhpattern.json` and `.mhprogram.json` are individual schema-versioned
    files; funscripts can import as a finite program or normalized pattern, and
    only pattern import compresses stationary gaps over five seconds.
17. **Smooth/harshen filters when auditioning patterns** (notes). *Covered in
    Phase 14*: Training offers temporary Original, Smooth, and Crisp audition
    filters. They alter only the resolved audition definition and never rewrite
    stored pattern points.
18. **Reference repos for pattern work** (notes): FunGen, FredTungsten
    Scripts, thehandy_resources, Howl (pattern generation/processing),
    OpenFunscripter, funscript-io, Funscript-Tools, HapticsEditor-v2,
    FunscriptDancer, funscripting, theedgy.app changelog (ideas). *Adopted*:
    recorded here as the Phase 14 research list (STGPT ROADMAP §13 has the
    fuller annotated set).

## D. Device scope

19. **Handy 2 / firmware differences / max-speed limits per device;
    Handy 2 Pro overclock selector** (notes). MagicHandy pins firmware v4 /
    API v3 (ADR 0006, R16) and has no per-device speed-limit model.
    *Adopted*: R16 gains a review item — check current Handy API docs for
    Handy 2 / 2 Pro deltas (including overclock) before packaging (Phase 16)
    claims device support; expose per-device max-speed limits only from
    documentation, never guesses.
20. **Commanded-estimate vs. live device position** (KNOWN_PROBLEMS).
    *Covered/Deferred*: MagicHandy labels the visualizer as a commanded
    estimate (parity row 4); comparing against a live position poll remains
    a hardware-session follow-up, same as STGPT's.

## E. UI affordances (design-conforming placements)

21. **Scrolling motion-history indicator** (notes: widen the motion
    indicator into a scrolling sequence of changes — motion, speed, etc.).
    *Deferred*: candidate diagnostics/Chat-page panel fed by the existing
    trace ring; must stay a readout (no controls), not crowd the
    visualizer, and follow dot+text conventions. Revisit with Pattern
    Library UI (Phase 14) where sequence visibility matters most.
22. **Per-slider "test this speed" affordance** (notes: play the device at
    the selected speed for ~3 s from setup). *Adopted*: small item alongside
    Phase 14/16 polish; the manual motion test endpoint already provides the
    backend path (`POST /api/motion/start` + timed stop).
23. **Hotkeys: "I'm close" and Stop; pause without reset** (notes).
    *Covered/Deferred*: Esc-stop and phase-preserving pause/resume shipped;
    an "I'm close" affordance (and its hotkey) arrives with the mode
    events that need it (Edge-style modes, post-Phase 14).
24. **Profile name/picture, About window with support links** (notes).
    *Deferred*: cosmetic; revisit at Phase 16/17 polish. Keep donation/support
    content in README (already public-oriented).

## F. Setup and diagnostics

25. **Verbosity levels that actually change what is shown** (notes: motion
    verbosity up to timing/latency/connection; LLM verbosity up to raw model
    output including thinking). *Partially covered*: diagnostics verbosity
    setting, trace ring, and trace export exist; raw-LLM-output visibility
    at the highest verbosity is *Adopted* as a diagnostics follow-up
    (display-only, never persisted into chat history).
26. **Backend logs to a file; stable CLI output** (notes). *Adopted*: small
    diagnostics item — optional `-log-file` flag (structured logs already go
    to stderr; a file sink is a flag away). Clickable URL is already printed.
27. **Silent failures / stale-tab UI** (notes). *Covered*: backend-
    authoritative state, offline banner + control lockout, toasts on save.

## G. Personalization (Phase 15 inputs)

28. **Personality presets, including the GLaDOS prompt** (notes). *Adopted*:
    Phase 15 import maps legacy personas onto MagicHandy prompt sets; the
    GLaDOS prompt ships as an importable example, not a bundled default.
29. **User identity / interest selector** (notes: startup + settings
    preference with self-ID and interest options, custom entries).
    *Adopted*: Phase 15 personalization item — feeds the prompt/memory
    system; optional, off by default, stored locally like all personal data.
30. **LLM-adjustable timing with an experimental opt-in** (notes).
    *Covered by direction*: this is Autopilot's job, bounded by arrangement
    segments and the quick-settings envelope; the "experimental, off by
    default" requirement matches the existing Autopilot safety contract.

## H. Already-confirmed equivalences (no action)

- Quick settings on the visualizer (STGPT PR #333) ↔ MagicHandy immediate-
  apply quick controls + active-motion refresh (Invariant 9).
- Settings UI refactor + prompt library (PR #328) ↔ Phases 10/11 shipped.
- Per-mode mode-action permissions (PR #326) ↔ already a Phase 13 UI rule.
- Freestyle "stops at regular intervals" (notes, major error) ↔ solved
  structurally (one continuous stream across segment retargets, zero stops,
  proven over the fake transport); real-hardware freestyle validation
  remains on the checklist.
- Timer/mode indicator split with fixed sizes ↔ status bar readouts.
- Chat/responsive shell, malformed-response visibility, repair pass ↔
  Phase 9/React shell.

## Relationship

- `IMPLEMENTATION_PLAN.md` (Phase 14/15 scopes reference this sweep)
- `docs/ui-design.md` (parity baseline; this sweep is its second pass)
- `docs/risk-register.md` R2 (two-codebase drift — this sweep is an R2
  mitigation artifact), R16 (device scope), R17/R15 (voice)
- StrokeGPT-ReVibed `Changelog.txt` PRs #319–#333, `KNOWN_PROBLEMS.md`,
  `ROADMAP.md` (2026-06/07 state), and the working notes file
- `docs/legacy-lessons-sweep-2026-07-11.md` (third pass: the full PR history
  #21–#333, skeptical dispositions of everything this sweep did not cover)
