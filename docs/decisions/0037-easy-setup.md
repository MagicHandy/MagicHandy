# ADR 0037: Easy Setup

Date: 2026-09-30

Status: Accepted

## Context

Guided setup has seven steps: welcome, access, device, chat AI, voice, install
and finish. Each exposes real choices (network access, three device transports,
four chat engines, several voice modules), and a first-time user rarely needs
most of them. The streamlining review asked for a path that works out what the
computer can run and asks only what a person has to decide.

## Decision

The Welcome step offers **Easy setup** (the default and recommended choice) or
**Custom setup** (the existing seven steps). Easy setup has four steps:
Welcome, Your setup, Install and Finish.

The backend assesses the machine (`assessment` in `GET /api/setup/status`) and
decides what it can run. The browser only words the verdicts.

| Feature | Met | Partly met | Not met |
| --- | --- | --- | --- |
| Chat | an NVIDIA GPU holds a curated model | NVIDIA found, memory unreadable | no NVIDIA GPU, too little memory, or not Windows x64 |
| Voice output (Chatterbox Turbo) | NVIDIA GPU | CPU only, speech is slower | installer not included |
| Voice input (Parakeet) | CPU, or already installed | — | installer not included |

The assessment also picks the model: the first catalog entry the GPU holds
(ADR 0034), or the one already installed. It reports the free disk space and the
space each feature needs, and the page warns when the selected features do not
fit.

Easy setup asks only:

1. How explicit chat should be (the four chat voice levels).
2. Whether to speak replies aloud (voice output).
3. Whether to talk instead of typing (voice input).
4. Optionally, the Handy connection key. Left empty, the device connects later
   from the top bar.

**Install and continue** saves those choices and submits the same backend
install plan as Custom setup: runtime, model download, Chatterbox on the
assessed device, and Parakeet, each only if needed or chosen. Voice turns on
when installed. Access stays local-only. Chat is skipped when its requirements
are not met, since CPU inference is too slow for live chat. The page says so
and points to Custom setup, which can connect a chat server on another
computer.

## Consequences

- A typical NVIDIA machine goes from first launch to chat with three answers
  and one click.
- Easy setup never exposes the network or picks an untested model. Everything
  it skips stays available in Settings and Custom setup.
- Chatterbox is the only voice output Easy setup installs, because it speaks
  immediately. Faster Qwen3-TTS needs a reference recording first, so it stays in
  Custom setup.
