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

The backend assesses the machine (`assessment` in `GET /api/setup`) and
decides what it can run. The browser only words the verdicts.

| Feature | Met | Partly met | Not met |
| --- | --- | --- | --- |
| Chat | an NVIDIA GPU holds a curated model | NVIDIA found, memory unreadable | no NVIDIA GPU, too little memory, or not Windows x64 |
| Voice output | Qwen3-TTS and chat fit on NVIDIA | memory unconfirmed, or Chatterbox CPU fallback | installer not included, or unsupported platform |
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
install plan as Custom setup: runtime, model download, the selected voice on the
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
- Qwen3-TTS is preferred on NVIDIA, with an explicit reference-recording step.
  Chatterbox offers an included voice and CPU fallback.

## 2026-10-05 amendment: voice preference and shared VRAM

The original implementation always selected Chatterbox for immediate speech,
without considering Qwen or the LLM/TTS combination's memory. Easy Setup now
prefers the pinned **Faster Qwen3-TTS 0.6B Base** unless the combined allowance
exceeds the detected NVIDIA GPU's total VRAM. An unknown amount of VRAM or an
unmeasured LLM keeps Qwen preferred with an unconfirmed-fit warning. A missing
installer or unsupported hardware still prevents installation.

The backend returns both voice options, device, disk estimate and memory verdict
in `assessment.voice_options`. For each CUDA option, required MiB is:

`max(LLM catalog minimum, LLM measured VRAM + voice allowance + 1024)`.

The voice allowances are **4096 MiB for Qwen 0.6B** and **2048 MiB for
Chatterbox Turbo**, with **1024 MiB for the desktop**. These are initial planning
allowances, not measured peaks or an upstream memory guarantee. They include
room beyond model weights but cannot account for every reference length, CUDA
graph allocation, driver, other GPU application or synthesis workload. The
catalog's LLM measurements apply at the default 32,768-token context; a larger
context is explicitly unconfirmed rather than extrapolated. Smaller contexts
keep the conservative default-context measurement. We do not sum multiple GPUs
or subtract live free memory, which could double-count an already loaded LLM.

If Qwen exceeds the budget, Chatterbox is recommended. If Chatterbox's GPU
allowance also exceeds it, Chatterbox runs on CPU. A skipped LLM reserves no
VRAM. A ready selected store model is kept; catalog measurements are used only
when its content hash matches, otherwise its LLM memory is marked unknown.
The assessment and install plan therefore describe the same selected model.

After enabling spoken replies, the page shows both modules with simple latency,
memory, CPU fallback and reference-voice tradeoffs. Choosing Qwen below its
allowance shows a non-blocking warning and still submits the ordinary CUDA
install plan. There is no separate force flag, override dialog or memory gate.
Selecting Qwen opens a two-field panel in the same step: a local WAV sample and
its exact transcript, with a visible mandatory-sample disclaimer. An explicit
**Set up voice later** action installs it without enabling spoken replies;
Settings > Voice can finish configuration. A saved reference is prefilled and
retained. There is no new wizard screen, microphone capture, upload or automatic
transcription. Both Easy and Custom setup use the same panel.

The backend checks the bounded local WAV's format and duration (16 MiB maximum,
PCM or floating-point, one or two channels, 1–30 seconds; 3–10 recommended).
It requires a trimmed non-empty transcript of at most 8192 bytes. The check
does not recognize speech or verify that the text matches. Host configuration
access is required before reading local files. Reference data travels in the
install plan, never in installer shell arguments, progress output or reports.
The current voice settings remain unchanged until the module installs.

Before applying the installed module, the reference is rechecked. Qwen spoken
replies turn on only when the install explicitly provided a validated reference
and requested voice enablement. Deferring configuration keeps replies off,
including when older reference fields exist. Existing module-update behavior
continues to preserve settings. Finish explains how to test or complete the
voice in Settings.

This is a deterministic recommendation policy, not a claim of experimentally
optimal speed/quality on every GPU. The pinned [Qwen runtime](https://github.com/andimarafioti/faster-qwen3-tts/blob/a70afc0f81f7f5f8801c3227968f1102f43f211c/README.md)
documents CUDA-graph streaming; the pinned [Chatterbox server](https://github.com/devnen/Chatterbox-TTS-Server/blob/915ae289340e10c6047f27f47e22eae9bf350c32/README.md)
documents Turbo and CPU operation. Representative simultaneous LLM/TTS peak
memory and listening benchmarks remain acceptance work; the UI labels the
numbers as estimates.
