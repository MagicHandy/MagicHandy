# ADR 0075: ElevenLabs account and catalog reads in the core

## Status

Accepted 2026-11-25 with the ElevenLabs refactor. An independent Gemini 4
Neon review proposed routing these reads through the speech worker instead.
That proposal is recorded below and declined.

## Context

Reports said ElevenLabs speech did not work. Live checks against the current
API found five causes:

- The default voice now resolves to a Voice Library voice, which free plans
  cannot use through the API.
- Restricted keys failed the worker's account probe, because ElevenLabs now
  answers a missing permission with `401`, not `403`.
- Turning on **Speak replies** did not start the worker.
- Timeouts of 30 and 60 seconds cut off long replies.
- Settings conflicts blocked key saves.

People also asked for more voices, especially voices suited to a companion
speaking to one listener.

Choosing a voice needs the account's voices, the Voice Library, the model list,
the plan and its credits, and previews. ADR 0003 keeps speech jobs behind the
JSON-lines worker protocol. That protocol carries one speech job at a time and
reports worker health. It has no request/response channel for catalog queries.

## Decision

1. **The core makes the read-only account and catalog calls.** These are the
   account check, models, voices, Voice Library search, adding a library
   voice, and previews, under `/api/voice/elevenlabs/*`. The speech worker
   keeps every speech request.
2. **One client package.** `internal/voice/elevenlabs` holds the HTTP client,
   error classification, retries, request shaping and text splitting. The
   core and the worker both use it, so their errors and limits agree.
3. **The routes are host-only.** Reads are cached per key fingerprint, at
   most two upstream calls run at once, and the key never leaves the core
   except in the worker's environment.
4. **Previews go through the core.** The browser never fetches ElevenLabs
   storage directly. The core rebuilds each preview URL on
   `storage.googleapis.com` and allows only ElevenLabs' public preview paths.
   It sends no credentials there.
5. **No new dependency.** Everything uses the Go standard library.

## Rejected alternative

Gemini proposed sending catalog queries through the worker so that only the
worker talks to ElevenLabs. That would add a query/response message family to
the ADR 0003 protocol. A settings page would have to wait for a worker that
might be stopped, starting or busy speaking. The isolation would be nominal
anyway: the core already holds the key in its settings store and passes it to
the worker. Isolation gains nothing when both processes hold the same secret.

## Consequences

- Opening Voice settings with ElevenLabs selected makes outbound HTTPS calls
  from the core to `api.elevenlabs.io`. No call is made while another provider
  is selected or before a key is set.
- The account check spends no credits. Adding a library voice changes the
  ElevenLabs account and requires the controller.
- The core binary carries the shared client package. Its cost is recorded in
  `docs/goal-scorecard.md`.
- A per-request voice or model override in the worker protocol is deferred.
  The worker still speaks with the saved voice and model.
