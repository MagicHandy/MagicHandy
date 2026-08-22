# LLM Latency Consistency

## Scope

This document records the interactive-chat latency policy introduced after
warm local-model requests were observed completing quickly while some complete
app turns took more than 15 seconds. The changes preserve the existing prompt,
sampling parameters, parser/repair policy, model-owned motion decision, and
shared motion path. They change runtime scheduling and observability only.

## Diagnosis

The 2026-08-01 Windows/NVIDIA investigation separated provider time from the
rest of the app path:

- direct production-prompt requests to the already-loaded managed Gemma model
  reached the first token in about 128-141 ms and completed in 553-654 ms;
- changing an early system-prompt prefix, which defeats llama.cpp prompt-cache
  reuse, raised individual requests to about 1.9 seconds but did not explain
  15-32 second turns;
- two persisted app chat turns took 33.811 and 32.090 seconds;
- the first cold Autopilot decision reached its 25-second timeout, while later
  decisions generally took 3-7 seconds;
- an active Faster Qwen CUDA synthesis roughly doubled a warm direct LLM probe
  from about 0.6 to 1.14 seconds, and canceled speech continued generating
  behind the HTTP request;
- a dead-parent `llama-server` process retained about 7.1 GB of GPU-backed
  working state alongside the live managed runner and TTS process.

The evidence points to several independent sources of tail latency: cold model
load, contention between autonomous and interactive model requests, shared-GPU
speech that did not stop producing after cancellation, and orphaned managed
runner processes. Prompt quality was not the limiting factor in these samples.

## Runtime Policy

### Managed model loading

Settings > Model exposes two managed llama.cpp policies:

- **At startup** (default): load in the background when the core starts. After
  readiness, preload sends one bounded 16-token, non-thinking JSON warmup so
  llama.cpp also performs its one-time generation-graph setup before the first
  user turn. The warmup uses autonomous scheduler priority: a waiting Chat turn
  cancels it and takes the model slot. The loaded model reserves RAM/VRAM while
  the app is idle.
- **On demand**: leave the model unloaded until requested. This saves idle
  memory, but the first request waits for the model to load.

Changing the runtime, selected model, context size, or load policy closes the
old managed provider before applying the new choice. Startup remains
non-blocking: the HTTP UI is served while preload runs.

The managed runner uses llama.cpp's low process/thread priority. Autonomous
prompt prefill can otherwise consume the host scheduler long enough for the
speech clock to appear to skip rendered seconds (the shorter motion clock is
hidden behind its `planned` state during the same inference). The backend
deadline remains authoritative either way; yielding to the app shell keeps the
countdown, Stop, and other safety controls responsive at a small potential cost
to generation latency.

### Interactive priority

One coordinator admits local-model generation calls. Chat and Autopilot still
use the same provider and the same contracts, but only one stream can occupy the
configured single llama.cpp slot at a time.

- a waiting interactive Chat turn runs before queued autonomous work;
- a newly submitted interactive turn cancels an in-flight autonomous inference,
  waits for that provider stream to release, and then starts;
- canceling an Autopilot inference does not stop Autopilot or alter motion. The
  mode scheduler simply plans again at its next valid opportunity;
- autonomous work never cancels an interactive turn.

This policy removes an avoidable full Autopilot timeout from interactive tail
latency without changing generated text or motion semantics.

### Speech interruption

When **Speak chat replies** is enabled, Settings > Voice exposes:

- **Interrupt current speech** (default): a new user message cancels active and
  queued TTS work before prompt preparation. This is preferable when TTS and the
  LLM share one GPU.
- **Finish current speech**: preserve current and queued speech. This can make
  playback more continuous, but the next local-model response may be delayed by
  GPU contention.

Completed speech history and retained playable audio are not removed by an
interactive interruption. Emergency Stop keeps its separate, stronger
invalidation behavior.

The managed Faster Qwen server now bounds its producer queue to two chunks,
propagates HTTP disconnect/cancellation to the producer, and closes the upstream
model generator. A CUDA kernel already in progress may finish its current chunk,
but abandoned utterances no longer continue through the remaining text.

## Managed Process Recovery

On Windows, every newly launched managed `llama-server` is assigned to a Job
Object with kill-on-close. Normal exit, app shutdown, and most abnormal parent
termination paths therefore release the child.

An older build or forceful system interruption may still leave a process. At
startup MagicHandy checks only for processes whose full executable path exactly
matches the currently configured app-managed runner. It does not match by file
name and never includes Ollama or an external llama.cpp path. A matching process
blocks a second managed launch and opens a confirmation dialog. The backend
re-reads the executable path immediately before terminating a selected PID, so
a stale dialog or PID reuse cannot target an unrelated process. Dismissing the
dialog leaves the process running and keeps the second launch blocked.

## Diagnostics

Persisted assistant-message diagnostics now separate:

- request preparation;
- model scheduler wait;
- time from request start to first token;
- initial provider generation;
- repair-provider time;
- total provider call count;
- total request time.

These fields use the existing message `diagnostics_json` document. The two new
preferences use the existing versioned settings document in SQLite. No table or
column migration is required, and older rows remain readable.

## Acceptance

The 2026-08-01 isolated complete-route run reused the already-loaded Gemma
llama.cpp endpoint through MagicHandy's external-provider mode while keeping
the test app on a separate database and fake transport. Voice and all model
motion capabilities were disabled. The first cache-fill turn completed in
1,836 ms with a 1,537 ms first token. The following seven turns completed in
379-614 ms (416 ms median), with first tokens in 164-441 ms (175 ms median).
Every turn used one provider call, required no repair, and followed the requested
short conversational tone. Three fresh stripped app launches reached health in
92.9-136.7 ms. Policy tests separately verify that startup schedules preload,
on-demand skips it and releases a cached provider, and exact-path termination is
controller-confirmed. Live shared-GPU Faster Qwen cancellation remains open; the
user's active voice process was intentionally not altered for this run.

The 2026-08-20 packaged-runtime run used the installed b9966 CUDA server,
Gemma-4-26B-A4B-Abliterated Q3_K_S, and a 32K managed context. Before generation
warmup, the first complete UI turn took 37,526 ms with its first token at
37,372 ms; the immediately following turn took 344 ms total with its first token
at 185 ms. Moving the one-time generation setup into startup preload took
73,747 ms from process start through model load and warmup without blocking the
HTTP UI. The first fresh user turn after that preload took 5,087 ms with its
first token at 4,961 ms, and the next turn took 309 ms total with its first token
at 183 ms. The remaining first-turn cost is the real production prompt prefill,
not another model load or repair. The isolated database had no connection key,
and both UI turns reported `motion=none`.

Before merging a latency change:

1. Compare repeated complete Chat-route turns, not only direct provider calls.
2. Include a warm baseline, an Autopilot-contention case, and a canceled-TTS
   case when the local speech module is available.
3. Report first-token, scheduler-wait, generation, repair, and total times.
4. Confirm no prompt, sampling, parser, authorization, or motion-target change
   entered the latency patch.
5. Verify startup preload and on-demand mode separately.
6. Verify a duplicate-path process is blocked, shown for confirmation, and only
   terminated after exact-path revalidation.
