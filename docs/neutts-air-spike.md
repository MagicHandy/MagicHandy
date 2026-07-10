# NeuTTS Air Spike (R17)

Phase 13.1. The question ADR 0007 left open: can NeuTTS Air's NeuCodec
decode path run **without Python**, and are cloning quality and latency good
enough to justify building the full MagicHandy worker? If not, the recorded
fallback is F5-TTS (ONNX) or an optional Python worker, with ElevenLabs as
the premium cloud path either way.

Spike date: 2026-07-09. Verdict: **the non-Python decode path is proven
viable; go ahead with the worker integration.** Details and evidence below.

## What NeuTTS Air is (as shipped today)

- A 748M-parameter Qwen2-backbone speech LM ("NeuTTS-Air", Apache-2.0) plus
  the NeuCodec neural audio codec (FSQ + Vocos + ISTFT, 24 kHz output).
- Official GGUF quantizations exist (`neuphonic/neutts-air-q4-gguf`,
  `neutts-air-q8-gguf`), designed to run through llama.cpp in real time on
  CPU — the same runner family MagicHandy already manages for chat (ADR
  0005), and deliberately off the LLM's GPU.
- Text is phonemized before the backbone; the official pipeline requires
  **espeak-ng** (a small native system package; `winget install -e --id
  eSpeak-NG.eSpeak-NG` on Windows).
- Voice cloning conditions generation on codec tokens encoded from 3–15 s
  of reference audio plus its transcript.

## The decode-path question (the R17 core)

The backbone half was never in doubt: llama.cpp runs the GGUF and emits
codec tokens. The risk was NeuCodec **decode** (tokens → waveform), whose
reference implementation is Python/PyTorch. Findings:

1. **Official ONNX decoder.** Neuphonic ships an ONNX decode path for
   NeuCodec explicitly to drop the PyTorch dependency (`neutts[onnx]`,
   `neuphonic/neucodec`). Any ONNX-runtime host can drive it — including a
   Go worker binary via onnxruntime bindings, or any C/C++/Rust host.
2. **Pure-CPU reimplementation exists.** A Rust port (`neutts` crate,
   v0.1.1, MIT, July 2026) implements the NeuCodec decoder as **pure Rust —
   FSQ + Vocos + ISTFT with no ONNX runtime at all** — on a GGUF backbone
   via llama.cpp bindings. This is proof the codec is small and simple
   enough to reimplement outside Python entirely.

Conclusion: **R17's feasibility half is answered — yes, non-Python NeuCodec
decode is real, twice over** (official ONNX artifact; independent pure-Rust
decoder). The unproven-tech risk collapses to ordinary integration work.

## Measured quality/latency (this spike's run)

Environment: Windows 11, CPU-only (the design goal: keep TTS off the LLM's
GPU), official `neutts` package (v1.2.1) as the measurement harness — the
same Q4 GGUF backbone + NeuCodec our worker will drive, so its output *is*
the quality/latency our worker would produce. espeak-ng installed via
winget; llama-cpp-python compiled locally by MSVC during pip install.

Measured 2026-07-09 (`neutts-air-q4-gguf` backbone via llama.cpp CPU;
NeuCodec decode on CPU; watermarking off):

| Step | Wall time | Audio out | RTF |
| --- | --- | --- | --- |
| Cold model load (incl. first-run HF downloads) | 190 s | — | — |
| Reference encode (13 s clip, once per voice) | 14.1 s | — | — |
| Short sentence | 1.88 s | 3.06 s | **0.62** |
| Two sentences | 2.96 s | 5.70 s | **0.52** |
| Paragraph (4 sentences) | 8.09 s | 16.02 s | **0.51** |

Read: **comfortably faster than real time on CPU** (RTF ≈ 0.5–0.6), so
sentence-level streaming gives a perceived first-audio latency of roughly
the first sentence's synth time (~2 s) and playback never starves once
started. Reference encoding is one-time per voice and belongs in `load`,
not per request. The cold-load figure is dominated by the one-time model
download (~0.7 GB) — an explicit user action in the worker design.
All outputs were verified as real 24 kHz audio (non-silent RMS); WAVs for
subjective cloning-quality listening are at
`scratchpad/neutts-spike/spike-{short,medium,long}.wav` (kept out of the
repo — audio artifacts are never committed).

Harness caveat: the measurement ran NeuCodec decode through the default
torch-CPU path, not the ONNX decoder; decode is a small share of the
pipeline (the backbone dominates), so the RTF transfers, and the ONNX/
pure-Rust decode paths above are the ones the worker will use.

Reference cloning used the official `samples/dave.wav` (~13 s) and its
transcript. NeuTTS 1.2.1 also offers optional Perth audio watermarking —
worth enabling by default in the worker (provenance without quality cost).

## Worker integration shape (historical spike design)

This section records the design proposed by the spike. The implementation
update below is authoritative where the available Rust port differs from the
proposal.

A separate **TTS worker binary** speaking the Phase 12 protocol
(`docs/voice-worker-protocol.md`), never part of the core binary — the core
stays `CGO_ENABLED=0` pure Go:

1. `load` starts a llama.cpp runner on the NeuTTS GGUF (reusing the managed
   llama.cpp pattern) and initializes the NeuCodec decoder; `unload` tears
   both down. Model files are explicit user downloads with visible sizes
   (goals-and-guardrails: no silent multi-GB pulls).
2. `speak` phonemizes (espeak-ng), prompts the backbone with the reference
   codes + text, collects codec tokens **per sentence**, decodes each
   sentence to WAV, and streams `audio_chunk` frames — sentence-level
   streaming falls out of the token stream naturally.
3. Reference voice: one configured reference WAV + transcript; codes are
   encoded once at load and cached.
4. Decoder host options, in preference order:
   - **Rust worker** using the `neutts` crate (pure-CPU decode, no ONNX
     runtime, llama.cpp linked in-process) — fewest moving parts at
     runtime; needs a Rust toolchain at build time and Windows validation
     (the crate documents x86_64 Linux first).
   - **Go worker + onnxruntime** (official ONNX decoder) — Go end to end,
     but the onnxruntime bindings are CGo, so the *worker* build needs a C
     toolchain (the core does not).
Either satisfies ADR 0003/0007; pick after the first Windows build of
the Rust crate is attempted.

## Implementation update (Slice 13.6)

The first integration uses a Go ADR 0003 adapter around the `neutts-rs`
`stream_pcm` executable. The runner emits live 24 kHz signed 16-bit PCM; the
worker forwards those chunks immediately and the core wraps retained PCM as a
WAV only when the controller fetches completed audio. Cancellation kills the
active runner process and final-sample preservation is covered by tests.

The crate's public `NeuCodecEncoder` in v0.1.1 is still a stub. Some examples
on its current branch describe a future Burn encoder, but the exported type
still rejects `new`, `load`, and `encode_wav`. MagicHandy therefore requires
pre-encoded `.npy` reference codes and the exact transcript. It stores the WAV
path only as provenance and does not claim arbitrary-WAV cloning without
Python. This is a narrower result than the original item 3 above, but it is
the capability the non-Python runner actually provides today.

All runner children receive `HF_HUB_OFFLINE=1`. Model artifacts and voice
codes must be installed explicitly before load/speech; status and speech can
never initiate a model download. A later Rust release with a working,
redistributable encoder can replace the `.npy` requirement behind the same
settings and worker protocol.

## Constraints hit during the spike

- Toolchain on this dev machine: **MSVC Build Tools present** (llama.cpp
  compiled fine during the run), **gcc/clang absent**. Consequence: the
  Rust worker candidate builds here (Rust uses MSVC on Windows) after a
  `rustup` install; a Go+CGo worker does not (CGo needs gcc/clang). This
  strengthens the Rust-worker preference. Workers ship as prebuilt
  binaries either way (R7 packaging).
- espeak-ng is a hard dependency of the official pipeline. It is small,
  winget-installable, and worker-side only — but it is a native system
  package the install docs must cover (installation-automation doc).
- The Python harness environment (~4 GB venv incl. torch) exists only in
  the session scratchpad for this measurement; nothing of it enters the
  product or the repo — the shipped worker has no Python.

## Fallbacks (unchanged, now less likely to be needed)

F5-TTS (ONNX) or an optional Python worker behind the same protocol;
ElevenLabs (next PR) covers expressive premium cloning regardless.

## Sources

- https://github.com/neuphonic/neutts (official; GGUF + ONNX decoder,
  espeak requirement, Windows notes, Apache-2.0 for Air)
- https://huggingface.co/neuphonic/neutts-air, `.../neutts-air-q4-gguf`,
  `.../neutts-air-q8-gguf`, `.../neucodec`
- https://docs.rs/neutts (pure-Rust decoder evidence, MIT, v0.1.1)

## Relationship

- ADR 0007 (backend selection), ADR 0003 (worker boundary), ADR 0005
  (llama.cpp runner pattern), `docs/voice-worker-protocol.md` (the protocol
  the worker implements), `docs/risk-register.md` R17.
