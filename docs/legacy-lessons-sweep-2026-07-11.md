# Third Legacy Sweep — Full StrokeGPT-ReVibed PR/Changelog History (2026-07-11)

Sources: the complete StrokeGPT-ReVibed `Changelog.txt` (PRs #21–#333) and PR
history, read end to end. The first sweep built the functional parity
baseline (`docs/ui-design.md`); the second
(`docs/legacy-parity-sweep-2026-07.md`) dispositioned the notes files and the
June 2026 hardware PRs (#319–#333). This pass mines the ~300 older PRs those
sweeps only sampled, and keeps only lessons **not yet considered** in
MagicHandy's docs or code.

## Reading these lessons skeptically

StrokeGPT-ReVibed lessons deserve suspicion before adoption, for reasons the
history itself demonstrates:

- **Many "lessons" compensate for that architecture, not for reality.** The
  chatty per-command REST design, three parallel motion backends, a polling
  browser UI, and the Python voice stack generated whole families of fixes
  (buffer tuning, pressure guards, echo dedup) that MagicHandy's design makes
  unnecessary. Adopting them would import the disease with the cure.
- **Motion patterning fixes stacked until they fought each other.** PR #316
  is the cautionary tale: the stop/go morphs it finally fixed were partly
  *caused* by earlier fixes (#301's shorter startup buffer + #306's
  conditional resume + #314's jitter guardrails), and the durable resolution
  **removed** guardrails rather than adding one more. Treat any legacy motion
  constant or compensation as a hypothesis needing single-variable,
  on-device evidence — never copy it forward as a rule.
- **Numeric defaults are one user's taste on one device.** The 12–45 s
  autospeak range, threshold curves, and buffer sizes were tuned by feel on
  a single Handy over one network. Adopt the *contract shapes*; re-derive
  the numbers.

Dispositions: **Adopt** (queue for a phase), **Verify** (plausible gap —
check before building), **Watch** (record; act only if evidence appears),
**Skip** (rejected, with the reason).

---

## A. Verified gaps (checked against MagicHandy code this pass)

1. **Browser audio vs the ASR engine's accepted formats** (PR #164).
   STGPT's Parakeet integration broke because browser recordings are
   WebM/Opus and the ASR path (torchaudio, there) silently lost the ability
   to decode them; the fix normalized everything to mono WAV before the
   engine. MagicHandy has the same shape of risk today:
   `internal/voice/parakeetworker` uploads the browser's WebM bytes verbatim
   as multipart to parakeet.cpp's `/v1/audio/transcriptions`, and whether
   that server decodes Opus has never been exercised — the plan's
   real-microphone Parakeet measurement is still pending. **Verify (high
   priority):** run mic → managed parakeet.cpp end to end; if the server
   wants PCM, decode/downmix to mono WAV host-side (pure-Go Opus decode is
   the hard part — evaluate before promising it) or capture WAV in the
   browser via an AudioWorklet. Skepticism note: the *torchaudio* framing is
   STGPT-specific; the transcendent lesson is only "never assume the ASR
   layer eats browser containers."
2. **Per-client bookkeeping must be bounded** (PR #275). STGPT found
   abandoned browser tabs growing server-side cursors and queues without
   limit. MagicHandy's `client_cursors` table has the same property: rows
   upsert per client id and are deleted only by full-table clear
   (`internal/chat/log.go`); fresh ids arrive whenever localStorage is
   cleared or a private window opens. Growth is slow (one small row per
   client) but unbounded and invisible. **Adopt:** prune cursor rows by
   `updated_at` age (e.g., on append, alongside the existing message-cap
   prune). The same PR's other half — keep the frequent status poll slim,
   full command history only in explicit diagnostics — is already
   MagicHandy's shape; keep it true as `/api/state` grows.
3. **Blocked sends must not eat the user's words** (PR #129). STGPT guarded
   chat sends on model readiness and, critically, *preserved the draft and
   the voice transcript* when a send was blocked. MagicHandy's `ChatPanel`
   clears the draft before streaming begins, so an LLM that is down or still
   loading consumes the typed message (and a push-to-talk transcript goes
   through the same `sendText`) leaving only an error bubble. **Adopt:**
   restore the draft on send failure, and consider a readiness disable on
   Send while the LLM runtime reports not-ready. Small, user-visible, cheap.

## B. Autopilot inputs (autospeak PRs #221–#228, #248, #266, #289)

MagicHandy's initial Chat Autopilot is in review in PR #101. It now uses
bounded canonical conversation context, publishes successful autonomous lines
through Chat, preserves custom content on hold, falls back visibly after bad
turns, and cancels announcements with Stop. The contract shapes still worth
carrying — not the old implementations — are:

4. **Bounded, user-configurable cadence with the model inside the bounds**
   (#222/#224/#228). The model chooses the next-speak delay, clamped to a
   user range; a zero floor caused immediate re-prompt loops, so zero maps
   to the minimum natural pause. Adopt the shape (model-chosen delay within
   user clamps, non-zero floor); re-derive the numbers — 12–45 s is taste.
5. **Autonomy ladder** (#289): Talk Only / Style Only / Full Motion levels
   for what autonomous turns may do. Adopt — it matches the existing
   per-mode action-permission stance (off by default, opt-in). **Still open.**
6. **Autonomous turns are real conversation; deterministic narration is
   not** (#225/#227): autospeak output routes through the normal
   chat/TTS/history channel and may be chat-only (`action: continue`,
   `move: null`); planner narration stays out of chat. MagicHandy already
   now holds this for Chat Autopilot: model lines use the canonical log and
   optional browser-playable TTS; deterministic fallback narration stays out.
7. **The loop must survive bad turns** (#266): failed or malformed
   autonomous turns schedule a retry at the minimum pause instead of dying
   silently; small local models need recent-line reminders to stop
   repeating themselves (#248). The initial slice keeps motion alive with a
   traced deterministic segment and includes recent lines; user-configurable
   retry/speech cadence remains open. Treat anti-repetition
   prompt scaffolding as a Watch — it is model-specific patchwork that a
   better model may not need.

## C. Motion and transport (read with the most skepticism)

8. **Differentiated retarget lead by intent class** (#259): user-visible
   intent changes got a shorter handoff lead than same-pattern drift
   updates, so deliberate changes feel snappy while background drift stays
   safe. MagicHandy uses one latency-aware lead policy. **Watch:** worth an
   experiment only after the Phase 14 real-device feel check, and it
   interacts with the dwell floor — do not stack both blind.
9. **Semantic no-op guard at the engine boundary** (#316/#258): repeated
   LLM outputs that *look* like changes (`middle` again with jittered
   numbers, `tip+flutter` noise on an unchanged request) kept flushing
   active streams into stop/go morphs. MagicHandy's affirmation test covers
   non-action chat, but not "same action, insignificantly different
   numbers." **Adopt (small):** an engine-level meaningful-change test —
   semantically equal targets must not retarget. This is the one #316
   lesson that transcends the HSP replacement contract.
10. **Play is for starts and recovery, not for retargets** (#316): replaying
    `/hsp/play` on every replacement restarted firmware playback and starved
    it a second later; stream/tail indexes had to reset within flushed
    batches. MagicHandy retargets in-stream without flush-replacement, so
    this does not currently apply. **Watch:** becomes load-bearing if a
    flush-style replacement or the Intiface pacer (Phase 14B) ever
    reintroduces a restart-shaped operation; the contract suite's "stop
    preemption ≠ playback restart" distinction should encode it then.
11. **Clamp/scale exactly once** (#301): requested speed was scaled through
    the user range twice, so high requests could never reach the configured
    cap. MagicHandy fixed the identical bug-shape for reverse direction
    (double inversion) and pins it with a test; speed has the single-clamp
    rule (ADR 0002) but no mirror regression test. **Adopt (small):** a
    speed-clamped-exactly-once test beside the reverse-direction one.
12. **First motion must not wait on a clock probe** (#301): the first play
    blocked behind a slow `/servertime` call. **Verify (small):** confirm
    MagicHandy's first `PlayHSP` does not synchronously probe server time on
    the user-visible start path; if it does, warm it in the background.
13. **Record device rate-limit headers** (#304): captured 429/rate headers
    in diagnostics settle "is the API pressuring us" arguments with data.
    **Adopt (small):** stash rate-limit headers in cloud transport
    diagnostics. Skip the rest of #304 (throttles and serialization exist
    to save STGPT from its own chatty design; MagicHandy batches by
    design and the engine already serializes dispatch).

## D. Device health eventing (#253–#255, #259)

14. **Subscribe to device health events, not just playback state.** STGPT's
    v3 SSE listener surfaced temperature, blocked-slider, low-memory,
    disconnect, and device-error events, cached (sanitized) in diagnostics —
    real-device stops became explainable without another command. MagicHandy
    reads HSP state/events on demand but keeps no persistent health
    subscription and has no vocabulary for these event classes. **Adopt** as
    a device-diagnostics slice (natural fit next to R16's Handy 2 review),
    keeping STGPT's own hedges: the poll stays as fallback, and SSE
    activity/failure is itself diagnosable. With it comes #259's
    serialization lesson: when command responses, polls, and SSE all update
    cached device state, order by device clock so stale snapshots never
    overwrite fresher ones.

## E. Voice input (hands-free inputs, mostly deferred with the slice)

15. **Microphone calibration is a contract, not a knob pile** (#97):
    persisted noise-suppression/echo-cancellation/AGC choices plus a
    room-noise RMS calibration that adapts the hands-free trigger threshold.
    The plan's voice UI rules already promise "one calibration/sensitivity
    path" — fold these specifics into the hands-free slice when it opens.
16. **Stage-timing diagnostics for the voice path** (#96): transcript→chat,
    LLM, and motion-apply timings exposed per request, so "voice feels slow"
    reports decompose. **Adopt-lite:** the voice request snapshots already
    carry timestamps; surface per-stage durations in diagnostics when the
    hands-free slice lands.
17. **Cached-only warm start** (#95/#272): preloading only *already
    downloaded* models at startup, never triggering downloads. MagicHandy's
    never-autostart invariant deliberately rejects the autostart half;
    the download/no-download distinction is already law (checksummed,
    explicit installs). **Skip**, noting the nuance is already covered by
    13.8's auto-load-on-user-start.

## F. Session pacing (#258, #259)

18. **Session intensity arc.** A steady/ramp-up/ramp-down/variable arc
    selector fed the elapsed session timer and an `arc` variable into LLM
    context, shaping long-session pacing; the arc timer reset after long
    idle gaps. Nothing in MagicHandy considers session-scale pacing.
    **Adopt skeptically** as a Phase 15 personalization / Autopilot input:
    the need (sessions have shape) is real; the mechanism must stay a
    *prompt-context input* to the existing planners — never a second motion
    path (R14) — and the idle-reset detail matters if adopted.

## G. Ops and packaging

19. **No-download setup verifier** (#271): a script checking runtime,
    reachability, ports, and writable folders without fetching models.
    **Adopt** as a Phase 16 packaging item (`magichandy -doctor` or a
    verifier script beside `install.ps1`).
20. **Offline capture replay** (#272): an analyzer for exported motion
    captures let hardware reports be studied off-device. **Watch** — trace
    export exists; build the analyzer the first time a real-device report
    needs one, not before.
21. **Whole-library export with a secrets-excluding manifest** (#272):
    Phase 14 shipped per-pattern share files; a one-click full-library
    export (patterns, programs, preferences, manifest, no secrets) is a
    small, user-visible addition. **Adopt-lite** for a Phase 14 follow-up.

## H. Considered and rejected (so the next sweep skips them)

- **Jitter guardrails and buffer/tempo tuning** (#247, #302, #303, #310,
  #314): superseded by the parametric catalog + PCHIP sampler (already
  adopted); #316 removed much of #314 on-device. Do not resurrect.
- **Queued-reply echo dedup machinery** (#32, #126, #248 echo half): the
  seq-numbered shared chat log (Phase 13.0) makes the whole bug class
  unrepresentable.
- **Chat/TTS divergence warning** (#141/#102): MagicHandy enforces
  byte-identical lockstep at append time; a divergence warning would be
  dead code.
- **Fenced-code chat rendering** (#143): wrong genre of chat app; plain
  text with `pre-wrap` is correct here.
- **Frozen elapsed-timecode UI fix** (#81): artifact of STGPT's
  frontend-computed log timecodes; MagicHandy trace rows carry their own
  timestamps.
- **HSP schema archaeology** (#183–#190): the v3/v4 schema wars are exactly
  why Phase 4's invariant tests and ADR 0006 exist; nothing left to mine.
- **Python voice stack hardening** (#160–#164 runtime isolation, #288–#295
  protobuf/numpy pins, Chatterbox #195/#265/#273/#274): the non-Python
  worker decision (ADR 0007) retired this entire maintenance genre; the
  one transcendent item is A1 above.

## Relationship to earlier sweeps

This document extends, and does not replace,
`docs/legacy-parity-sweep-2026-07.md` (items 1–30) and the functional parity
baseline in `docs/ui-design.md`. Adopted items land as small slices inside
existing phases: A1–A3 near-term, B with the Autopilot slice, C/D alongside
device work, E with hands-free, F with Phase 15, G with Phases 14/16.
