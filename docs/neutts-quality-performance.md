# NeuTTS Quality and Performance Evaluation

Date: 2026-07-15

This evaluation covers the app-managed NeuTTS Air Q4 path after the eSpeak NG
and overlap-aware codec corrections. Tests used the official 7.45 second Dave
reference, its exact transcript and 372 reference codes, the pinned CUDA/WGPU
runtime, and managed Parakeet for an intelligibility round trip. ASR can expose
missing or substituted words, but it cannot certify naturalness, speaker
similarity, or the absence of mild slurring; those still require listening.

## Causes

### Per-request random sampling

Pinned `neutts-rs` v0.1.1 applies top-k 50, top-p 0.9, temperature 1.0 sampling
and chooses a fresh random seed for each generation when no seed is set. This
made the same text produce different speech tokens, pacing, and stop points.
Twelve warm requests for one sentence produced 4.60-9.10 second clips even
though Parakeet recovered nearly all words. A fixed-seed sweep also found an
extreme 13.70 second result, showing that duration alone is not stable.

A four-sentence corpus rejected candidate seed 5 after one 17-word sentence
stopped at 0.14 seconds with an empty Parakeet transcript. Seed 10 garbled the
time/name phrase. Seed 3 completed all cases and Parakeet retained all target
words. The managed runner now uses seed 3 by default. `--seed random` or
`MAGICHANDY_NEUTTS_SEED=random` restores upstream randomness for custom/direct
runner diagnostics; the app-managed worker explicitly pins the manifest seed.
Random mode disables PCM caching because repeated output is expected to vary.

This removes intermittent variation for identical inputs. It does not prove
that seed 3 is optimal for every reference voice or sentence, so representative
listening remains part of R17 exit evidence.

### Reference conditioning

Reference quality remains an independent cause of consistently weak output.
Neuphonic recommends a clean, natural 3-15 second sample with little noise and
few pauses. Transcript mismatches condition the wrong words even when reference
codes are valid. MagicHandy's encoder normalization, exact-transcript workflow,
and preview reduce this risk but cannot repair clipped, noisy, multi-speaker, or
incorrectly transcribed source audio.

### Audio assembly

The quality-corrected runner used the right lookback/lookahead overlap math, but
it retained every decoded frame and recomputed the full overlap-add result after
each chunk. That repeated prior work and grew per-request memory with clip
length. The runner now retains only the not-yet-final overlap tail and emits the
same finalized samples incrementally. A pinned Rust test compares the new path
sample-for-sample with full recomputation; four generated corpus WAVs were also
SHA-256 identical before and after the change.

### Playback completion delay

The browser still plays a completed retained clip rather than live worker PCM.
It previously polled completion every second, adding 0-1000 ms after synthesis.
The poll interval is now 250 ms. True incremental browser playback needs a
lease-gated PCM stream and a cancel-safe Web Audio scheduler; it is a separate
contract change, not something to approximate with partial WAV files.

## Speedups

- The resident runner keeps model/reference state loaded across requests.
- Incremental overlap-add removes repeated full-history mixing while preserving
  exact PCM. Unique-request inference remains dominated by the backbone and
  codec, so short-clip wall time changed only modestly.
- A process-local exact-text LRU stores at most eight successful clips, 2 MiB
  each and 8 MiB total. Its lifetime naturally scopes entries to the active
  model, reference, sampler, and runner configuration. Canceled, failed,
  oversized, and random-seed requests are not cached.
- A measured repeated 4.70 second clip went from 1.91 seconds of warm synthesis
  to a 0 ms cache replay in the harness. The replay WAV was byte-identical.
- Browser completion polling now adds at most about 250 ms instead of one
  second before the completed clip is fetched.

The cache is deliberately memory-only. A disk cache would retain generated
voice audio, need model/reference/version invalidation metadata, create cleanup
and privacy obligations, and provide little value for mostly unique chat text.

## Deferred work

- Sentence splitting may limit long-form model drift, but every segment repeats
  prompt prefill and independently conditions prosody. Joining those clips can
  introduce audible seams. Add it only after matched long-form listening.
- Best-of-N generation plus ASR scoring multiplies latency and still cannot
  score timbre or mild slurring reliably.
- The prompt places variable input phonemes before the reference speech-code
  suffix. Reusing a simple contiguous KV prefix therefore does not avoid most
  prefill; a deeper pinned-upstream context-cache patch is not justified yet.
- Lower temperature or greedy decoding changes the upstream generation recipe
  and needs a broader voice/reference listening study before replacing the
  deterministic seed approach.
- Incremental browser playback is the largest remaining unique-utterance
  latency opportunity. It requires tests for controller lease changes,
  Emergency Stop, queue ordering, underruns, and final-tail delivery.

## Acceptance evidence

- Four mixed seed-3 corpus cases: 3.82-8.56 seconds of audio, 1.37-2.73 seconds
  synthesis, and 0.52-0.63 seconds to first runner PCM on the RTX 5070 Ti host.
- Parakeet retained all target words for all four seed-3 clips.
- All four optimized-runner WAVs exactly matched their pre-optimization seed-3
  SHA-256 hashes.
- Exact-text cache miss/hit: 1.91 seconds then 0 ms; equal WAV SHA-256.
- A clean full-feature updater run rebuilt schema 4 to schema 5 in 10 minutes
  56 seconds while preserving the saved llama.cpp, Ollama, Parakeet, and NeuTTS
  choices. Through the relaunched production HTTP path, an uncached request
  reached `done` in 2.799 seconds and its exact repeat in 34 ms. Both returned
  the same 277,484-byte WAV and SHA-256, and the shared queue returned to zero.
- Pinned Rust runner tests, Go adapter/API tests, frontend playback tests, and
  installer manifest tests cover the implementation boundaries.

Primary references: the pinned
[`neutts-rs` sampler](https://github.com/eugenehp/neutts-rs/blob/v0.1.1/src/backbone.rs)
and Neuphonic's
[`neutts-air-q4-gguf` model card](https://huggingface.co/neuphonic/neutts-air-q4-gguf).
