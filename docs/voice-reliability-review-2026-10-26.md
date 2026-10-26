# Voice reliability and resource-pressure review — 2026-10-26

## Scope and baseline

Started from the latest local branch, `claude/stop-network-error` at
`ad5450c2`, including its uncommitted Stop/network fixes. Those changes are
preserved in `codex/voice-reliability`; they are not attributed to this review.
The report concerns intermittent cut-off speech and apparent startup errors
with managed Faster Qwen3-TTS and Parakeet after alpha.74's llama.cpp update.
The reporter has an RTX 4070 Ti with 12 GB VRAM. Their chat model, reference
audio and seed mode are unknown.

No direct voice adapter or playback change was found in the alpha.73–74
release diff. That does not exclude a shared-resource effect from the LLM.
The measurements below use an RTX 5070 Ti with 16 GB VRAM, the pinned installed
Qwen 0.6B Base runtime, Parakeet TDT 0.6B v3 Q4_K, and a synthetic reference.
Allocator limits emulate reduced memory headroom, not a complete 4070 Ti.
User-owned app processes, credentials and voice references were not copied or
stopped. No real device was connected or moved.

Claude Opus 5.5, CLI effort `xhigh`, independently reviewed and implemented the
Voice settings redesign, then reviewed the lifecycle changes. The integration
review added operation ownership, crash retention and background health fixes
from that feedback.

## Confirmed defects and changes

1. **Normal cold loading could look like failure.** Some workers load
   synchronously on the protocol reader. A three-second health probe sent
   behind that operation timed out, overwrote the model state and produced
   `context deadline exceeded`. The supervisor now owns a tracked model
   operation; status reads return loading/unloading immediately. Concurrent
   equal requests share it. Closing one caller, or saving an unrelated voice
   setting, does not orphan the load. Stop/process exit cancels the owned
   operation, and connection/revision fencing rejects stale results.
2. **A managed process can be alive but unable to speak.** A real injected CUDA
   allocation failure mid-stream left the Qwen process alive while subsequent
   replies repeatedly failed. Managed stream interruption now clears readiness
   and retains a runtime failure. Explicit retry/reload replaces the owned
   child. It does not silently replay an utterance or restart an external
   service; intentional cancellation does not mark the model failed.
3. **Startup readiness was too weak.** Managed Qwen now requires
   `model_loaded=true`. A false ready field is a loading condition for an
   owned server, not an immediate failure. Invalid health contracts still
   fail. Parakeet retains unexpected child-exit evidence across status polls
   until a successful load or intentional unload. Empty stderr buffers are
   supported.
4. **Lifecycle races could hide the actual failure.** Failed startup no longer
   unconditionally stops whichever process is current by the time the HTTP
   handler returns. A crash remains crashed through idle cleanup. Background
   health checks on the existing connection monitor recover transient probe
   errors and detect failed children even while Voice settings is closed.
5. **Brief browser read failures abandoned otherwise healthy voice jobs.**
   Audio/status/transcript reads now retry transient network/502/503/504
   failures at the same offset, with at most three retries and 200/400/800 ms
   backoff. Submission, expired permissions and cancellation are not retried.
   Abort interrupts both polling and backoff.
6. **Support evidence was insufficient.** Diagnostic status now includes
   retained voice-request timing, byte/chunk counts, terminal state and
   whitelisted error categories. Failed and canceled requests retain elapsed
   time. Spoken text, transcripts, audio, reference audio and provider error
   bodies are excluded. This history is bounded and in-memory; it does not
   recover voice requests from an earlier app run.

## Measured behavior

| Scenario | Observation |
| --- | --- |
| Baseline Qwen short / paragraph / long | Complete 4.32 / 20.16 / 40.88 s audio, synthesized in 1.56 / 6.29 / 12.58 s. Parakeet recovered the final “lighthouse” in each. |
| Cold Qwen startup | First observed load 43.67 s; later process starts mostly 16–19 s. A working cold load must not be presented as a failed module. |
| Same E4B 16K chat load on llama.cpp b11149 vs b11533 | Three simultaneous Qwen paragraph requests each: median 12.205 vs 12.099 s. All six WAV files byte-identical to the baseline paragraph. No direct regression reproduced. |
| Qwen process limited to 2 GiB PyTorch allocation | Startup failed after 26.86 s; subsequent health continued to report the failure. |
| Qwen process limited to 3 GiB | Loaded in 18.81 s; three replies completed in 1.61 / 1.54 / 1.43 s. About 2365 MiB allocated and 2776 MiB reserved at readiness. |
| Qwen limited to two logical CPU threads | Loaded in 18.60 s; three replies completed in 1.61–1.67 s; no cut-off. |
| Mid-speech CUDA pressure | A temporary 2.65 GiB allocation cap plus an attempted 512 MiB allocation interrupted the first reply after 92,204 WAV bytes. Removing the pressure did not repair that process: later replies also failed, down to header-only output. |
| Same injection after the fix | Health correctly reported a runtime failure. Explicit reload took 15.83 s; the next three replies completed in 1.42–1.47 s, each 207,404 bytes and byte-identical to the successful short baseline. |
| Parakeet, two logical CPU threads | Short / paragraph / long transcription: 0.89 / 4.20 / 9.07 s, compared with 0.17 / 0.77 / 1.72 s normally. All final words retained; subsequent normal process returned to baseline. |

Qwen's memory figures are for that speech process, not a recommendation that
3 GiB total VRAM is sufficient. Chat model weights, KV cache, other apps and
CUDA allocations also consume memory. The constrained tests alter only the
test-owned process; they do not fill global VRAM or interfere with user apps.

First-audio timing in the initial protocol harness included the WAV header and
is not a playable-audio latency measurement. The later pressure/warm-up harness
measures first PCM instead. The shorter baseline and recovery results use
fixed seed 1337 to make errors and waveform comparisons reproducible.

## Warm-up alternative evaluated and rejected

Compared the production one-frame warm-up with consuming a short full sentence,
in A/B/B/A order, with three spoken replies after each cold start. Full-sentence
warm-up added roughly 0.9 seconds to startup. First PCM improved only about
15–20 ms (0.379–0.383 s vs 0.363–0.368 s), and every output was byte-identical.
The existing warm-up is retained. This small clean-reference trial does not
establish that every reference or Varied seed behaves identically.

## UI and interaction review

Voice settings now has separate Graphite input/output cards with restrained
borders, status words/dots, appropriate actions and collapsed technical details.
Loading is amber, readiness green, and real failures remain visible. Long
tracebacks are not placed above the form. Start/test use saved settings;
Stop/Cancel remain usable with unsaved edits. A draft provider change labels
the currently running saved provider. Status fetch failures distinguish initial
unavailability from a stale last-good snapshot.

Operation epochs prevent late Start/Cancel/Test results from clearing a newer
action or queueing speech after Stop. The Chat microphone and quick controls
explain loading rather than offering another Start while one is in progress.

Live review at 390 px and the desktop viewport confirmed no horizontal
overflow, accessible Stop, loading/cancel/restart transitions and a completed
Qwen sample through the browser (311,084 bytes, 2.19 s synthesis). The isolated
app passed a real Ollama Granite 3B generation probe. The final refreshed build
was verified by binary hash and rechecked: Parakeet processed the silent UI test
in 23 ms (correctly reporting no speech), Qwen generated the complete sample in
2.09 s with a successful browser playback acknowledgement, and both workers
returned to Ready. Twenty-four status reads spanning cold loading and
readiness completed in 3–44 ms (5.63 ms mean) with no false failure; ten
observed Loading. The final browser console reported no warnings or errors.

## Validation and reproducibility

- Full frontend: 130 test files, 1061 tests; typecheck, all five locale audits
  and canonical production build pass.
- Full Go tests, full `go test -race ./...` and vet pass; lint passes with zero issues. Pure-Go Windows
  production core and both worker builds pass.
- Seven Python streaming tests pass, covering disconnect cleanup, bounded
  producer queues, cancellation, private API restrictions and conditioning.
- Regression coverage includes slow startup, caller cancellation, coalescing,
  stale results, crash evidence, automatic health recovery, managed stream
  failures versus deliberate cancellation, browser read recovery and privacy.
- Browser chat with motion Off returned a non-empty Granite 3B reply in
  149 ms on the final build without repair/fallback. Motion remained idle.
  The working final app is left at `http://127.0.0.1:50301/#/settings/voice`
  with both voice modules ready, Speak replies off and simulated devices.

Local evidence and executable experiment harnesses are retained under the
ignored `.scratch/voice-review/`: `real-voice-results.json`,
`coexistence-results.json`, `warmup-results.json`, `pressure-results.json`,
`transient-pressure-results.json`, `transient-recovery-results.json`,
`parakeet-pressure-results.json`, test logs, WAVs and UI captures. The original
startup regression failure and the pressure-induced failures are retained.
The first Parakeet pressure harness incorrectly read a top-level transcript
instead of `candidates[0].text`; its failed harness log is retained, and the
corrected run supplied the measurements above.

Remaining acceptance: the reporter's exact 4070 Ti/chat-model/reference
combination, varied seeds and extended listening, Firefox microphone capture
and playback, and a real WAN audio interruption. The deterministic browser
tests cover retry/cancel behavior but are not claimed as a real WAN test.
The results establish reproducible defects and tested recovery, not a proven
single cause for every reported cut-off.
