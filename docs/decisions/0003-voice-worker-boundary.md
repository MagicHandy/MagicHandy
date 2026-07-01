# ADR 0003: Optional Voice Worker Boundary

## Status

Accepted for the rewrite plan.

## Context

Voice input and output are useful features but have caused much of the Python dependency pain in StrokeGPT-ReVibed. Chatterbox, faster-whisper, Parakeet, NeMo, Torch, CUDA, and related packages are large, platform-sensitive, and frequently affected by dependency conflicts.

MagicHandy should not make those dependencies part of the core app startup path.

## Decision

Voice input and voice output run behind optional worker boundaries. The Go core owns worker lifecycle, queueing policy, cancellation, status display, and UI/API contracts. Voice model runtimes can remain separate processes, including Python processes, as long as they implement a versioned worker protocol.

The core app must run without voice workers installed.

The concrete engines selected for the first implementation (Parakeet ASR,
NeuTTS Air local cloning TTS, ElevenLabs cloud TTS — all non-Python) are in
ADR 0007. Python workers are an optional later addition, not the default.

## Worker Protocol Requirements

Every worker protocol must include:

- protocol version
- provider name and version
- health/status request
- model load status
- model unload support when practical
- request ID
- cancellation by request ID
- timeout behavior
- queue depth reporting
- structured error payloads
- safe logging that excludes secrets and large binary payloads

## Worker Lifecycle

The Go core may support:

- disabled worker state
- manually started worker
- app-managed worker process
- crash detection
- restart policy
- graceful shutdown
- stderr/stdout log capture

Workers must report missing dependency errors clearly. A missing or failed voice worker must not prevent chat, settings, motion, transport, or diagnostics from working.

## TTS Contract

A TTS worker receives text and voice settings and returns audio chunks or a structured error. It must support cancellation. The Go core owns playback queue policy and must prevent catch-up flooding by exposing queue depth and allowing old audio to be dropped when configured.

## ASR Contract

An ASR worker receives audio or a file/stream reference and returns transcript candidates plus confidence/metadata. It must be able to reject no-speech, silence, or low-confidence audio without sending empty transcripts into chat.

## Message And Audio Delivery Ordering

Voice output is coupled to chat delivery; the old app sometimes spoke a reply the
chat panel never displayed. The core must keep these ordered:

- chat text emit and TTS enqueue happen in lockstep: a reply that is spoken is
  always also displayed
- clients read chat from a shared log via per-client cursors, not a destructively
  drained global queue, so one client cannot consume another's messages
- audio playback uses a single-owner lease (the active controller) so multiple
  clients never speak the same clip at once
- model/transport errors return to the initiating client as a visible error and
  stay out of chat history, TTS, motion, and persona/turn countdowns
- do not expose more voice tuning controls by default before good defaults are
  validated on real microphones; advanced controls stay collapsed or in
  diagnostics

## Consequences

Positive:

- The core app can be distributed as a small binary.
- Voice dependencies can be installed only by users who want them.
- Provider failures become isolated and visible.

Negative:

- IPC and worker lifecycle add complexity.
- Provider feature parity must be tested through protocol contracts.
- Packaging must decide whether workers are separate downloads, bundled options, or documented manual installs.
