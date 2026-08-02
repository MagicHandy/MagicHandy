# ADR 0005: Local LLM Runtime Strategy

## Status

Accepted. Provider adapters, the app-owned checksum-verified llama.cpp release
installation and process lifecycle, SQLite-backed model inventory, ID-based
selection, and explicit GGUF/Ollama import are implemented. Curated model
downloads remain planned.

## Context

MagicHandy depends on local LLM behavior for chat, motion intent extraction, JSON compliance, prompt repair, and future autonomous motion planning. The rewrite is prioritizing quality first, while still preserving broad platform reach.

StrokeGPT-ReVibed currently depends on Ollama for local model serving. Ollama is useful because it is cross-platform and already familiar to many users, but it also hides some runner details behind an external daemon and leaves MagicHandy with less direct control over model files, quantization, launch arguments, health, GPU settings, and curated app-specific defaults.

llama.cpp is a better fit for a quality-first Windows/NVIDIA path because MagicHandy can manage a specific runner, curated GGUF models, launch options, context size, and hardware-fit checks directly. That does not mean llama.cpp should be linked into the Go core or become the only supported path.

## Decision

MagicHandy uses a local LLM provider interface with two first-class providers:

- primary provider: managed llama.cpp for Windows/NVIDIA systems
- secondary provider: Ollama for cross-platform compatibility and externally managed models

The llama.cpp path is primary because it is the path MagicHandy can tune most tightly for quality, JSON reliability, predictable startup/load behavior, and future binary releases on the main supported platform. Ollama remains important as the compatibility pathway for Linux, macOS, CPU-only users, unsupported GPU stacks, and users who prefer to manage models outside MagicHandy.

The Go core must not link libllama or require CGo for the early implementation. It should manage `llama-server` as an external process and communicate over localhost using the OpenAI-compatible HTTP API. This preserves the pure-Go core and cross-build guardrail while allowing llama.cpp to evolve independently.

Managed mode does not accept runner or GGUF paths in settings. MagicHandy embeds
an installer pinned to llama.cpp `b9966` / commit
`c749cb041706647f460bb918cccc9d91995205ab`. An explicit controller action (or
the interactive installer) downloads the official Windows x64 CPU archive or
the CUDA 12.4 runner and runtime archives, verifies fixed sizes and SHA-256
digests, rejects unsafe archive paths, probes the exact commit, and atomically
activates a constrained manifest under the app data directory. CUDA activation
also requires the staged runner to detect a CUDA device. Startup and status
checks only inspect that manifest; they never download or install a runtime.

The managed installer needs PowerShell 5.1 and HTTPS. It does not install Git,
CMake, Visual Studio, MSYS2, or the CUDA Toolkit. CUDA requires only a
compatible NVIDIA driver and GPU. The app ships the pinned upstream MIT license
beside the helper and writes archive URLs, sizes, and digests to an app-owned
provenance manifest. Valid earlier `built_from_source` manifests remain
accepted so updates do not discard a working runtime.

Users may decline the managed installation and use an existing Ollama installation.
That avoids the managed runtime and, unless the user explicitly imports a
model, avoids duplicate model storage. This choice is functional, not a
degraded fallback: Ollama retains provider health, model listing/selection, and
streaming chat through the same orchestration layer.

## Provider Contract

Every local LLM provider must expose:

- provider identity and version
- availability/status check that does not download models
- installed/available model listing
- explicit load/unload when supported
- streaming chat completion
- cancellation or request timeout
- a provider-neutral output-token cap and reasoning policy, mapped only to
  fields the selected provider supports
- structured error payloads
- prompt/response metadata needed for malformed-response UI
- diagnostics that exclude secrets and large prompt bodies by default

Chat orchestration, JSON validation, repair prompts, prompt sets, and motion-target application stay above the provider boundary. Providers return text/stream data; they do not produce raw motion commands.

The current request policy defaults to a 256-token output cap for the app's
compact intent contract. Reasoning is explicit: `off` is the recommended
small-model default request and asks for provider-native thinking suppression; `auto` delegates to the
model, with the current pinned managed llama.cpp bounding hidden reasoning to
half the total budget through its pinned API. Repair always uses `off` and retains the original
conversation. Arbitrary external GGUF/Ollama models do not share one capability
contract, so suppression remains best-effort there. Hardware/runtime flags
remain managed defaults until measurements justify user-facing controls, with
one bounded exception: managed llama.cpp context size is durable and selectable
as 16,384, 32,768 (default), 65,536, or 131,072 tokens. Larger values allocate
more RAM and VRAM; values below the prompt length cannot fit a request. This
setting becomes `--ctx-size` only for the app-owned process, while external
llama.cpp and Ollama retain full ownership of their context configuration. Warm
managed requests do not repeat readiness and model-list probes after a
successful load.

Managed loading is an explicit resource/latency choice. The default `startup`
policy loads the selected app-owned model asynchronously after the core begins
serving; `on_demand` keeps idle RAM/VRAM free and makes the first request pay the
load cost. Interactive and autonomous generation share one admission
coordinator. Waiting Chat work overtakes queued Autopilot work and cancels an
in-flight autonomous inference before taking the single managed server slot;
autonomous work never preempts Chat. Per-message diagnostics separate
preparation, scheduler wait, first token, generation, and repair time. See
`docs/llm-latency-consistency.md`.

Model inventory is a sibling concern, not part of `Provider`. A provider is a
configured runtime adapter; the model manager owns durable records, managed
copies, imports, and filesystem state. This keeps model setup usable even when
no provider can be constructed because the current model selection is missing.

## llama.cpp Runtime Model

The llama.cpp provider manages:

- pinned verified release, app-owned runner activation, and version/backend metadata
- runner version and acceleration metadata
- localhost port selection
- process startup/shutdown
- Windows Job Object containment and exact-path duplicate-process recovery
- health checks
- model load errors
- stderr capture for diagnostics
- saved context-size launch allocation with a reviewed finite catalog
- GPU/VRAM fit warnings where practical
- timeout and crash handling

Initial target:

- Windows/amd64
- NVIDIA GPU acceleration
- curated GGUF models chosen for instruction following, JSON reliability, and the app's prompt style

Do not attempt to bundle every llama.cpp acceleration backend in the first implementation. CPU, Vulkan, ROCm, Metal, Linux, and macOS llama.cpp paths can be added later if they become worth the packaging and support cost. Ollama covers broad compatibility until then.

The current installer supports Windows/amd64 CPU and CUDA. `auto` chooses CUDA
when a working NVIDIA driver and GPU are detected; otherwise it installs CPU.
CPU downloads one approximately 18 MiB archive. CUDA downloads the official
CUDA 12.4 runner and runtime archives, approximately 628 MiB compressed and
1.1 GiB installed. This preserves one provider/manifest contract while removing
the multi-gigabyte compiler-toolchain failure surface from ordinary installs.

## Ollama Runtime Model

The Ollama provider remains supported but secondary. It should:

- connect to a user-managed Ollama daemon
- list installed models
- report unavailable daemon/model states clearly
- stream chat through the same provider contract
- use the same prompt, JSON repair, malformed-response, and motion application logic as llama.cpp

Ollama should not be removed just because llama.cpp becomes the primary path. It is the escape hatch for unsupported platforms, non-NVIDIA systems, users with existing Ollama libraries, and users who do not want MagicHandy to manage model files.

An explicit Ollama-to-managed import may read a user-selected Ollama model
library. It parses content-addressed manifests read-only, accepts only a
single self-contained GGUF model layer, copies that layer into MagicHandy's
store, and verifies SHA-256 against the manifest. MagicHandy never mutates the
Ollama library and never points a durable managed selection directly at an
Ollama-owned blob. See `docs/model-management.md`.

## Model Downloads And Management

MagicHandy must not download multi-GB models automatically during startup, setup checks, provider status checks, or first chat.

Model installation is explicit:

- show model name, source, quantization, size, license, checksum, context window, and expected hardware fit
- download to a temporary/incomplete path
- support resume when practical
- verify checksum before install
- move atomically into the model store
- allow cancel/retry/remove
- support importing a local GGUF file
- support copying compatible existing Ollama GGUF layers without requiring a
  second network download

Model metadata and UI expectations are detailed in `docs/model-management.md`.

## Consequences

Positive:

- quality-first path can be tuned for MagicHandy's prompts and JSON needs
- fewer hidden daemon assumptions on the primary Windows/NVIDIA path
- model files, context, runner flags, and diagnostics become visible product state
- Ollama remains available for broad platform support
- the Go core keeps its pure-Go binary story

Negative:

- MagicHandy now owns more Windows/NVIDIA runner packaging complexity
- CUDA and driver compatibility become visible support concerns
- model download UX, disk cleanup, checksums, and licenses must be handled carefully
- llama.cpp behavior can change across runner versions, so runner pinning matters
- maintaining two providers increases test surface

## Revisit Criteria

Revisit this decision if:

- managed llama.cpp cannot be made reliable for the Windows/NVIDIA audience
- Ollama quality and diagnostics become clearly better than the managed llama.cpp path
- CGo/libllama provides a measured benefit large enough to justify losing the pure-Go core guardrail
- cross-platform binary releases become more important than Windows/NVIDIA quality before Phase 17
