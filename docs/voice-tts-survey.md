# Voice TTS Survey: Fast, Cloning, Expressive

Survey date: June 2026. This is a landscape report to inform ADR 0007 (voice
backend selection). It is scoped to MagicHandy's constraints:

- **voice cloning** (zero-shot, from a short reference)
- **expressiveness** — either contextual (inferred from reference/text) or via
  **explicit emotion tags / control**
- **fast** — small time-to-first-audio, streamable
- **fits a 12 GB RTX 5070 that is also running the LLM** (so ~5-6 GB free, or CPU)
- **prefer non-Python** (the core stays pure-Go; workers are external processes)

Numbers are approximate and hardware/source-dependent. The 5070 is new, so treat
GPU figures as model-class estimates. RTF = compute time / audio length
(lower is faster; <1 is faster than real time). TTFA = time-to-first-audio.

## The one structural insight

The models that matter divide by **deployment path**, and that path decides the
Python question far more than the model does:

- **Tier A — speech-LLM + neural codec** (Orpheus, OuteTTS, Llasa, Spark-TTS,
  Kani-TTS, NeuTTS Air): the language model is a Llama/Qwen backbone that already
  runs on **llama.cpp as a GGUF** — the exact external runner MagicHandy is
  building for chat. Only the small **codec decoder** (SNAC / NanoCodec /
  NeuCodec / WavTokenizer / XCodec2) needs a native (C/C++/Rust) implementation.
  This is the non-Python sweet spot.
- **Tier B — ONNX-exportable** (F5-TTS, OpenVoice v2; plus non-cloning
  Kokoro/Piper via sherpa-onnx): run via onnxruntime from C++/Go, no Python.
- **Tier C — Python/PyTorch multi-stage** (CosyVoice, Dia, Chatterbox, Zonos,
  IndexTTS-2, MegaTTS3, CSM): the most expressive + best cloning, but Python.
- **Tier D — cloud API** (Cartesia, ElevenLabs, Hume): non-Python over HTTP,
  lowest latency, best expressive-cloning together — but not local/private.

## Tier A: speech-LLM + codec (non-Python via llama.cpp)

| Model | Size / VRAM | Speed | Cloning | Expressiveness | Notes |
|---|---|---|---|---|---|
| **NeuTTS Air** | 748M (Qwen); CPU or CUDA/WGPU | upstream claimed CPU real-time; MagicHandy measured 66.72x RTF on CPU and 1.39x RTF on CUDA/WGPU | 3-sec instant | Contextual (moderate) | On-device, 24 kHz, NeuCodec. CPU saves VRAM but was not interactive on the test host; persistent GPU first audio measured 0.47-1.01 s |
| **Kani-TTS-2** | 400-450M / ~3 GB | near-instant (NanoCodec) | Yes | Contextual (moderate) | Edge-optimized, low latency, tiny |
| **Orpheus-3B** | 3B (Llama-3.2) / Q8 ~6 GB, Q4 ~4 GB | TTFA ~180-300 ms | Yes, but **tone/timbre only** (loses prosody; repetition glitches) | **Explicit tags** `<laugh>`/`<sigh>` (8), near-human — but tags weaken during cloning | Apache-2.0, GGUF widely available. Expressive with *stock* voices |
| **Llama-OuteTTS-1.0** | 1B / small | ~real-time | Yes | Contextual (moderate) | WavTokenizer codec; earlier version of this pattern |
| **Llasa** | 1B/3B/8B / scales | AR (slower at 3B+) | Yes | Contextual (good at 3B+) | Llama + XCodec2; quality scales with size |
| **Spark-TTS-0.5B** | 0.5B (Qwen2.5) / small | fast | Yes | Contextual (good) | BiCodec; strong quality for size |

## Tier B: ONNX-exportable (non-Python via onnxruntime)

| Model | Size / VRAM | Speed | Cloning | Expressiveness | Notes |
|---|---|---|---|---|---|
| **F5-TTS** | ~0.35B / small | **RTF 0.33 (fastest)** | **Best clone fidelity (4.5/5)** | **Flat** — no prosody/emotion model | Non-AR flow-matching. The faithful-but-unexpressive option |
| **OpenVoice v2** | small | fast | Yes | Emotion/tone-color control | Base TTS + tone-color converter; community ONNX for C++/C#; modest quality |
| Kokoro-82M / Piper | tiny / CPU | sub-0.3 s | **No cloning** | Fixed voices | The instant default; runnable via sherpa-onnx (Go binding) |

## Tier C: Python/PyTorch (most expressive + best cloning, but Python)

| Model | Size / VRAM | Speed | Cloning | Expressiveness | Notes |
|---|---|---|---|---|---|
| **CosyVoice 3** | ~0.5B LM+flow / 4.5 GB | RTF ~0.52-0.63; streaming ~150 ms | Yes, cross-lingual | **Instruction control** (emotion/style/dialect) | Fastest emotion-capable; fits alongside LLM; Apache-2.0 code |
| **Dia-1.6B** | 1.6B / 6.2 GB | RTF 0.77 (compiled) | Yes | **21 real audio-event tags** (laughs/cries/gasps) | Expressiveness leader; tight VRAM alongside LLM |
| **Chatterbox(-Turbo)** | 0.5B (Llama) / 5.1 GB (fast) | RTF 0.50 fast / 1.67 std | **5.0/5, beat ElevenLabs in blind test (65% vs 25%)** | Configurable "exaggeration" dial | Best raw clone fidelity; MIT |
| **Zonos-v0.1** | 1.6B / ~4-6 GB | AR, moderate | Yes | **8-D emotion vector** (most controllable) | Apache-2.0; strongest explicit dial control |
| **IndexTTS-2** | / **14 GB** | RTF 0.80 | Yes | **Disentangled** emotion vs speaker (independent) | SOTA WER/sim/emotion — but **won't fit alongside the LLM**; text-handling bugs |
| **MegaTTS3** | 0.45B / small | fast | Ultra-high-quality | Some control | ByteDance; **cloning encoder is access-restricted** (safety gate) |
| **CSM (csm-1b)** | 1B / small | moderate | Yes | Well-rounded, controllable | Apache-2.0; Sesame pivoted to hardware |

Also rule out on a 12 GB card next to the LLM: **Higgs Audio v2** (14-16 GB).

## Tier D: cloud APIs (non-Python via HTTP; not local)

| Service | Speed | Cloning | Expressiveness | Notes |
|---|---|---|---|---|
| **Cartesia Sonic-3** | **TTFA 40-90 ms** (fastest) | Yes | Nonverbal tags (laughter/breath) + inflection | Streaming websockets, 15+ langs |
| **ElevenLabs** (Flash/v3) | ~75 ms Flash | Instant + Pro | Audio tags + strong emotion | Best-known; quality benchmark |
| **Hume Octave** | streaming | Yes | **Auto emotional context + NL instructions** ("sound sarcastic", "whisper fearfully") | LLM trained jointly on text+speech+emotion tokens |

## Expressiveness taxonomy (what "expressive" means per model)

- **Explicit audio-event tags:** Dia (21), Orpheus (8), ElevenLabs v3, Cartesia,
  Bark. Good when you want literal laughs/sighs/gasps.
- **Emotion dial / vector:** Zonos (8-D), Chatterbox (exaggeration), IndexTTS-2
  (disentangled). Good for global tone control.
- **Instruction / auto-context:** Hume Octave, CosyVoice, Qwen3-TTS. Natural-
  language style direction.
- **Contextual only (from reference prosody):** NeuTTS Air, Kani-TTS, Spark,
  CSM, XTTS, F5 (weakest). Inherit the reference's delivery; no independent
  emotion control.

## The persistent wall

There is still **no mature, local, non-Python model that does expressive *and*
faithful cloning at once**. Each non-Python-feasible option drops one thing:

- F5 (ONNX): faithful clone, flat delivery.
- Orpheus (llama.cpp): expressive stock voices, but cloning is tone-only and
  loses expressiveness/glitches.
- NeuTTS Air / Kani-TTS (llama.cpp): fast, small, real cloning, but only
  contextual expressiveness (no emotion tags/dials).

Expressive-tags-with-a-cloned-voice together remains a **Python (Tier C)** or
**cloud (Tier D)** capability today.

## Recommendations for MagicHandy

1. **Instant default (no clone):** Kokoro or Piper via sherpa-onnx (Go binding,
   also serves Whisper ASR) — voice works on day one, CPU, zero GPU contention.
2. **Best non-Python cloning, low-footprint (prototype first):** **NeuTTS Air** —
   Qwen backbone on llama.cpp, 3-sec cloning, and an effective persistent
   CUDA/WGPU path. CPU avoids LLM GPU contention but is a compatibility fallback,
   not an interactive default on the measured Windows host. **Kani-TTS-2** is the
   close alternative (3 GB GPU).
3. **Expressive tags, non-Python, local:** **Orpheus-3B** on llama.cpp — best for
   *stock* expressive voices; accept weaker cloning. Reuses the chat runner.
4. **Faithful clone, non-Python, local:** **F5-TTS (ONNX)** — accept flat delivery.
5. **Expressive + cloned together, non-Python:** **cloud** — Cartesia Sonic-3
   (fastest), Hume Octave (most expressive), ElevenLabs (best-known).
6. **Optional Python worker (best local expressive-clone):** **CosyVoice 3**
   (fast, fits 4.5 GB, instruction control), **Dia** (max emotion tags), or
   **Zonos** (8-D emotion dial). Never required; opt-in only.

**Suggested path:** ship the Kokoro/Piper default now, prototype **NeuTTS Air**
as the local non-Python cloning voice, wire a **cloud provider (Cartesia/Hume)**
as the expressive-clone premium, and leave a **CosyVoice3/Dia** optional Python
worker as the "max local expressiveness" escape hatch. Re-evaluate Orpheus/Kani
cloning as their codecs get native decoders.

Two levers still dominate perceived latency regardless of model: **sentence
streaming** (speak sentence 1 while sentence 2 renders) and **persistent model
preload**. GPU coexistence remains a capacity tradeoff: CPU can avoid contention,
but only when its measured latency is acceptable. See ADR 0003, "Message And
Audio Delivery Ordering".

## ASR (voice input)

Cloning/expressiveness are TTS concerns; ASR is judged on accuracy, speed, and
noise tolerance. Parakeet is no longer Python-locked — mature non-Python runners
now exist, which changes the pick.

| Model (non-Python runner) | English WER | Speed | Noise tolerance |
|---|---|---|---|
| **Parakeet-TDT-0.6B-v3** (parakeet.cpp / achetronic-Go / sherpa-onnx) | ~6.3% avg (beats Whisper); ~tie clean | Fastest — non-AR TDT, RTFx ~3000x | **No silence hallucination**; slightly weaker on heavy accent/noise |
| **Whisper large-v3-turbo** (whisper.cpp) | ~7.4% avg | Slowest (autoregressive) | Most robust to noisy/accented; **hallucinates on silence**; 99 languages |
| **Canary-1B-v2** (sherpa-onnx) | Best (beats Whisper-large-v3, ~10x faster) | Fast | Most noise-robust; heavier (1B), AED (some hallucination risk); 25 langs |
| **Moonshine** (sherpa-onnx) | Lower on hard audio | Very fast, tiny | English, edge/low-latency |

Non-Python runners: **parakeet.cpp** (C++/GGML, CUDA/Metal/Vulkan/CPU, full model
family, C-API/FFI), **achetronic/parakeet** (Go server, ONNX Runtime,
OpenAI-compatible, CPU+CUDA), and **sherpa-onnx** (Go bindings; runs
Parakeet/Canary/Whisper/Moonshine + Kokoro TTS behind one integration). All run
as external processes, so the Go core stays pure-Go — same pattern as the LLM
runner.

For an English-first hands-free app, **no silence hallucination** is the decisive
noise-tolerance trait — it was the exact failure the old Whisper-based hands-free
stack kept patching. Pick: **Parakeet-TDT-0.6B-v3 via sherpa-onnx** (swappable
engine), Whisper optional for heavy noise/accents or extra languages, Canary
optional for max accuracy. See ADR 0007.

## Sources

- [TTS-Model-Comparison-Chart (benchmarks)](https://github.com/mirfahimanwar/TTS-Model-Comparison-Chart/)
- [Best TTS Models 2026 benchmark (MarkTechPost)](https://www.marktechpost.com/2026/05/30/best-text-to-speech-tts-models-in-2026-a-benchmark-based-comparison/)
- [Best Local TTS Models 2026 (Local AI Master)](https://localaimaster.com/blog/best-local-tts-models)
- [Best open-source voice cloning 2026 (SiliconFlow)](https://www.siliconflow.com/articles/en/best-open-source-models-for-voice-cloning)
- [Kani-TTS-2 (MarkTechPost)](https://www.marktechpost.com/2026/02/15/meet-kani-tts-2-a-400m-param-open-source-text-to-speech-model-that-runs-in-3gb-vram-with-voice-cloning-support/)
- [NeuTTS Air / on-device TTS (GetStream)](https://getstream.io/blog/best-on-device-tts-models/)
- [Orpheus 3B GGUF (Unsloth)](https://huggingface.co/unsloth/orpheus-3b-0.1-ft-GGUF)
- [Orpheus vs ElevenLabs (Codersera)](https://codersera.com/blog/orpheus-3b-vs-eleven-labs-best-tts-model-compared/)
- [Cartesia Sonic](https://www.cartesia.ai/sonic)
- [Zonos TTS](https://www.zonostts.net/)
- [MegaTTS3 (ByteDance)](https://github.com/bytedance/MegaTTS3)
- [sherpa-onnx TTS models](https://k2-fsa.github.io/sherpa/onnx/tts/all/)
- [awesome-ai-voice list](https://github.com/wildminder/awesome-ai-voice)
- [TTS leaderboard / vendors (CodeSOTA)](https://www.codesota.com/text-to-speech)
- [parakeet.cpp](https://github.com/mudler/parakeet.cpp)
- [achetronic/parakeet](https://github.com/achetronic/parakeet)
- [Parakeet vs Whisper (Local AI Master)](https://localaimaster.com/blog/parakeet-vs-whisper)
- [Best open-source STT 2026 (Northflank)](https://northflank.com/blog/best-open-source-speech-to-text-stt-model-in-2026-benchmarks)
- [Canary-1B-v2 & Parakeet-TDT-0.6B-v3 paper](https://arxiv.org/pdf/2509.14128)
- [sherpa-onnx ASR engine](https://deepwiki.com/k2-fsa/sherpa-onnx/2.1-automatic-speech-recognition-(asr)-engine)

## Relationship

Feeds ADR 0007 (voice backend selection). Cloning + expressiveness are TTS-only;
ASR is covered in the ASR section above (Parakeet-TDT-0.6B-v3 via a non-Python
runner). See ADR 0003 for the worker boundary and delivery-ordering rules.
