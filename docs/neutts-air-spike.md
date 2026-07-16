# NeuTTS Air Spike (R17)

Phase 13.1. The question ADR 0007 left open: can NeuTTS Air's NeuCodec
decode path run **without Python**, and are cloning quality and latency good
enough to justify building the full MagicHandy worker? If not, the recorded
fallback is F5-TTS (ONNX) or an optional Python worker, with ElevenLabs as
the premium cloud path either way.

Spike date: 2026-07-09. Verdict: **the non-Python decode path is proven
viable; go ahead with the worker integration.** The feasibility verdict still
holds. The CPU latency inference did not survive implementation and is corrected
below.

> **Post-implementation correction (2026-07-15):** the Python measurement
> harness below was not performance-equivalent to the pinned Rust runner. The
> installed CPU runner measured 66.72x RTF and 90.86 seconds to first audio, not
> 0.5-0.6x RTF. Do not use the spike's CPU timing as a product expectation. The
> persistent CUDA/WGPU implementation measured 0.47-1.01 seconds to first audio
> on the RTX 5070 Ti host; full evidence is in `docs/perf-baseline.md`.

## What NeuTTS Air is (as shipped today)

- A 748M-parameter Qwen2-backbone speech LM ("NeuTTS-Air", Apache-2.0) plus
  the NeuCodec neural audio codec (FSQ + Vocos + ISTFT, 24 kHz output).
- Official GGUF quantizations exist (`neuphonic/neutts-air-q4-gguf`,
  `neutts-air-q8-gguf`) for llama.cpp. Upstream described CPU real-time use,
  but MagicHandy's pinned Windows runner did not reproduce it.
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

Environment: Windows 11, CPU-only, official Python `neutts` package (v1.2.1)
as the measurement harness. It used the same Q4 model family but a different
runtime stack from the later pinned Rust runner; the timing did not transfer.
espeak-ng was installed via WinGet and llama-cpp-python compiled locally by
MSVC during pip install.

Measured 2026-07-09 (`neutts-air-q4-gguf` backbone via llama.cpp CPU;
NeuCodec decode on CPU; watermarking off):

| Step | Wall time | Audio out | RTF |
| --- | --- | --- | --- |
| Cold model load (incl. first-run HF downloads) | 190 s | — | — |
| Reference encode (13 s clip, once per voice) | 14.1 s | — | — |
| Short sentence | 1.88 s | 3.06 s | **0.62** |
| Two sentences | 2.96 s | 5.70 s | **0.52** |
| Paragraph (4 sentences) | 8.09 s | 16.02 s | **0.51** |

Read as historical harness evidence only: this Python path was faster than real
time (RTF approximately 0.5-0.6), but the pinned Rust CPU implementation was
not. Reference encoding is one-time per voice and belongs in `load`, not per
request. The cold-load figure was dominated by the one-time model download
(~0.7 GB), an explicit user action in the worker design.
All outputs were verified as real 24 kHz audio (non-silent RMS); WAVs for
subjective cloning-quality listening are at
`scratchpad/neutts-spike/spike-{short,medium,long}.wav` (kept out of the
repo — audio artifacts are never committed).

Harness caveat: the measurement ran through llama-cpp-python and the default
Torch CPU codec, not the pinned `llama-cpp-4`/pure-Rust path. The original claim
that its RTF would transfer was incorrect; Slice 13.10's direct runner and
worker measurements supersede it.

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

The crate's public `NeuCodecEncoder` in v0.1.1 is still a stub. Some examples on
its current branch describe a future Burn encoder, but the exported type still
rejects `new`, `load`, and `encode_wav`. That does not require a Python fallback:
MagicHandy now builds a separate GPL-3.0-only Rust worker around the pinned
Apache-2.0 DistillNeuCodec ONNX encoder. It parses and bounds WAV input,
downmixes/resamples to the encoder's 16 kHz contract, and writes int32 NPY. The
Go boundary then re-parses and range-validates that output before storing it.

The older pure-Go preparation path remains for manual pre-encoded input. It can
safely extract the single contiguous int32 tensor used by official sample-style
Torch ZIP `.pt` files or validate a one-dimensional int32 `.npy` without
executing pickle.

All runner children receive `HF_HUB_OFFLINE=1`, and the adapter supplies the
exact Air Q4 filename to avoid repository discovery. Follow-up audit found that
the pinned upstream `hf-hub` client does not enforce that environment variable;
a missing cache could otherwise initiate network access on synthesis.
MagicHandy now requires the external decoder and exact GGUF cache entry before
starting the worker. Load probes `stream_pcm --help` for the required CLI
contract rather than running an expensive synthesis. It does not advertise an
offline capability: a network-denied integration test and hard sandbox remain
R17. A later upstream Rust encoder can be evaluated against the same managed
WAV and NPY contract without changing the Go core or settings model.

The Slice 13.6 adapter originally started one `stream_pcm` process per speech
request, so the upstream example's preload-once performance did not carry
across replies. Slice 13.10 supersedes that lifecycle with the persistent runner
described below; legacy custom executables retain the one-shot path.

A 2026-07-15 run using the official Dave sample normalized 372 codes and
produced 101,760 bytes of valid PCM. The process took 2m2.576s and emitted its
first audio after 87.98s on this CPU. The pinned runner also wrote a 93-byte
`NeuCodec decoder:` diagnostic to stdout before the PCM; the adapter now strips
only that bounded known prefix. This evidence supports a five-minute synthesis
job timeout, not a model-heavy readiness probe. Later audit found that this run
also used an inaccurate experimental phonemizer, so it is not quality evidence.

A 2026-07-15 reference-encoding follow-up used the 7.45 second, 44.1 kHz stereo
Dave WAV. The native ONNX worker produced 373 bounded codes in about 1.3 seconds.
The installed `stream_pcm` runner accepted those generated codes and produced
106,560 PCM bytes (2.22 seconds of audio) for a 122-token test. First audio took
93.51 seconds and total synthesis took 128.37 seconds on this CPU. This proves
encoder/runner format compatibility; it does not close subjective cloning
quality or the repeated model-start latency risk.

## Implementation update (Slice 13.10)

The installer now builds a first-party persistent runner against the exact
pinned source. CPU builds use system eSpeak NG and the upstream CPU codec. CUDA builds add
an exact patch that sets every llama.cpp backbone layer for GPU offload and use
the Burn WGPU codec. The app-managed schema-5 manifest records that choice, the
verified phonemizer/version, and checksums all five required CUDA llama/ggml
DLLs.

The worker starts this runner during model load and exchanges bounded framed
commands and PCM over standard streams. Backbone, codec, reference codes, and
reference transcript stay loaded across requests. Request cancellation does not
discard the model; unload, shutdown, and Emergency Stop terminate the child.

On the RTX 5070 Ti development host, a direct instrumented CPU run took 127.27
seconds wall time, with first audio at 90.86 seconds and a 66.72x real-time
factor. The CUDA/WGPU one-shot build took 5.28 seconds wall time, loaded in 1.90
seconds, reached first audio in 2.06 seconds, and completed synthesis in 2.45
seconds. Through the persistent Go worker, first request TTFA/total were
1.01/2.18 seconds and warm request TTFA/total were 0.47/1.17 seconds. A live
cancellation after the first audio chunk reached `canceled`; the same process
then completed a recovery request with 96,960 PCM bytes and exited cleanly.
These measurements close repeated startup and interactive-latency concerns on
the tested GPU. A quality audit then found that the pinned pure-Rust phonemizer
mispronounced common words and dropped text, while independent 25-token codec
decodes inserted discontinuities and long silence. Replacing it with eSpeak NG
1.52 and Neuphonic's 50-token lookback, 5-token lookahead, overlap-add stream
produced four clips whose Parakeet round trips retained every substantive target
word; two were exact sentence transcriptions. Subjective clone fidelity,
network denial, and shared-LLM VRAM acceptance remain open.

## Constraints hit during the spike

- Toolchain on this dev machine: **MSVC Build Tools present** (llama.cpp
  compiled fine during the run), **gcc/clang absent**. Consequence: the
  Rust worker candidate builds here (Rust uses MSVC on Windows) after a
  `rustup` install; a Go+CGo worker does not (CGo needs gcc/clang). This
  strengthens the Rust-worker preference. Workers ship as prebuilt
  binaries either way (R7 packaging).
- The historical Python harness used system eSpeak. Direct comparison proved
  that pinned `neutts-rs`'s experimental pure-Rust replacement did not reproduce
  it: common words were emitted as the wrong phonemes and one reference word
  disappeared. The installer therefore provisions eSpeak NG 1.52 and the runner
  invokes it directly without Python.
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
