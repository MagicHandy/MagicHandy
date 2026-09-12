# TTS reliability review — September 12, 2026

## Report and scope

A user reported `Chatterbox Turbo installation failed: exit status 1`, then
speech pausing every few words with Qwen3-TTS on a Ryzen 5950 / RTX 3070 machine.
The preceding installer output, installed app version, and throughput with the
LLM idle were unavailable. The exact installation failure is therefore unknown;
the hardware description alone does not establish the cause of the audio gaps.

This pass covers the Windows module installer, Go setup error propagation,
Python streaming adapters, PCM decoding, and browser speech scheduling. It adds
no dependencies, changes no model/package pins, and does not alter motion or
transport behavior. The user-installed app and optional modules are not updated
by this review.

## Confirmed defects and changes

1. **The process boundary hid the failing installer operation.** PowerShell
   already knew which native step failed, but Go replaced that detail with the
   outer process's generic exit status. Native environment/dependency/probe,
   source, and model-download failures now emit structured operation labels and
   exit codes. Go accepts only known labels for the current failed job. The
   operation survives restart without persisting arbitrary child-process output.
2. **Explicit Chatterbox CUDA installs delayed driver verification.** The early
   runnable NVIDIA driver probe previously applied only to Qwen. It now also
   covers explicitly selected Chatterbox CUDA before environment creation and
   dependency downloads. CPU installation remains available without NVIDIA.
3. **On-time PCM chunks could gain artificial silence.** Every chunk previously
   required a new 35 ms lead even if the preceding chunk was still scheduled.
   The scheduler now adds that lead only at initial start or a real underrun.
   The near-boundary regression schedules an arriving chunk at 0.835 s instead
   of inserting an extra 20 ms gap at 0.855 s.
4. **Immediate playback magnified uneven delivery.** Playback now collects
   0.75 s of audio initially. After an underrun it refills 1.5 s, then at most
   3 s. Short completed clips flush immediately, with no silence added and no
   samples dropped. This trades initial/recovery latency for continuity; it
   cannot compensate indefinitely for inference slower than real time.
5. **Cancellation could remain blocked on the producer.** Stop now interrupts
   pending iterator reads and a suspended audio context, and does not await a
   stalled producer's cleanup. Per-source cleanup is idempotent. PCM decoding
   also avoids whole-packet copies when no incomplete data is pending; retained
   partial bytes remain owned by the decoder. Mono playback uses a bulk copy.

## Installation investigation

The exact pinned Chatterbox server source is
[`915ae289`](https://github.com/devnen/Chatterbox-TTS-Server/tree/915ae289340e10c6047f27f47e22eae9bf350c32).
Its Windows Python 3.10 CPU and CUDA 12.1 requirements both resolve successfully
with the current MagicHandy constraints. A fresh workspace-only Python 3.10.20
environment installed the CPU requirements, pinned
[`chatterbox-v2` engine](https://github.com/devnen/chatterbox-v2/tree/cc0357396d9c73fc1e6c544ee40bb596020edd09),
the existing ONNX/tokenizer pins, and protobuf 4.25.8. The actual bundled native
runtime probe then reported `Verified Python voice runtime (CPU).`

This verifies CPU package installation and native imports. It does not verify
model downloads, synthesis quality, CUDA runtime execution, or coexistence with
an LLM on the reporter's GPU. An index/dependency-resolution hypothesis did not
reproduce, so this pass does not change dependency pins or weaken uv's index
selection policy. The existing Qwen conditioning cache already avoids repeated
reference encoding; no speculative re-encoding change was made.

## Validation and measurements

- Full Go tests and race tests, vet, lint, architecture/lifecycle gates, and a
  `CGO_ENABLED=0` app/worker build pass.
- Frontend typecheck, localization audit, all **518 tests in 71 files**, and the
  canonical production build pass. New cases cover short replies, growing and
  capped refill reserves, near-boundary scheduling, pending-read/unlock
  cancellation, idempotent cleanup, and reused input buffers.
- A deterministic delivery fixture supplies 24 chunks of 300 ms audio at
  alternating 550/50 ms intervals. After an initial 600 ms collection period,
  all 7.2 s of audio is scheduled contiguously. This is a scheduler regression,
  not a listening or inference benchmark.
- Windows PowerShell 5.1 installer fixtures pass, including a real native
  process that writes stderr and exits 73. Go tests verify fragmented output,
  malformed/untrusted labels, current-job identity, cancellation, and failure
  restoration without raw logs. Seven lightweight Python adapter tests pass.
- Node 24.15.0, 256 × 32 KiB raw mono PCM chunks, five warm-up iterations and
  eleven measured iterations per version, alternating version order: decoder
  median **7.97 → 5.80 ms**. The output checksum matches. These component timings
  do not predict TTS generation speed or speaker latency.
- Stripped app **19,218,432 → 19,224,576 B**; main JS gzip-9
  **211,890 → 212,097 B**; total raw embedded assets **2,030,099 B**. One isolated
  simulator observation: **599.1 ms** cold start, **33,918,976 B** idle working
  set (three samples), **57,327,616 B** private memory. Existing waivers remain.

## Review app

The current-source app runs with isolated data at
`http://127.0.0.1:49977/#/settings/voice`. Voice is off, Chatterbox CPU is selected,
and the optional model runtime is not installed in this isolated data directory.
The app includes current Go voice workers and installer helpers. Motion is routed
to the simulator; LLM motion and Autopilot are off.

`scripts/check-review-llm.ps1` passes against the exact app, using the available
local Ollama model `huihui_ai/granite4.1-abliterated:3b`. A visible text-only app
chat returns “The review is ready.” in 118 ms (62 ms first token, one provider
call), without repair/fallback or motion. Model speech listening and RTX 3070
inference throughput remain open acceptance work.
