# ADR 0005: Local LLM Runtime Strategy

## Status

Accepted for the rewrite plan.

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

## Provider Contract

Every local LLM provider must expose:

- provider identity and version
- availability/status check that does not download models
- installed/available model listing
- explicit load/unload when supported
- streaming chat completion
- cancellation or request timeout
- structured error payloads
- prompt/response metadata needed for malformed-response UI
- diagnostics that exclude secrets and large prompt bodies by default

Chat orchestration, JSON validation, repair prompts, prompt sets, and motion-target application stay above the provider boundary. Providers return text/stream data; they do not produce raw motion commands.

## llama.cpp Runtime Model

The llama.cpp provider manages:

- runner discovery or bundled-runner selection
- runner version and acceleration metadata
- localhost port selection
- process startup/shutdown
- health checks
- model load errors
- stderr capture for diagnostics
- GPU/VRAM fit warnings where practical
- timeout and crash handling

Initial target:

- Windows/amd64
- NVIDIA GPU acceleration
- curated GGUF models chosen for instruction following, JSON reliability, and the app's prompt style

Do not attempt to bundle every llama.cpp acceleration backend in the first implementation. CPU, Vulkan, ROCm, Metal, Linux, and macOS llama.cpp paths can be added later if they become worth the packaging and support cost. Ollama covers broad compatibility until then.

## Ollama Runtime Model

The Ollama provider remains supported but secondary. It should:

- connect to a user-managed Ollama daemon
- list installed models
- report unavailable daemon/model states clearly
- stream chat through the same provider contract
- use the same prompt, JSON repair, malformed-response, and motion application logic as llama.cpp

Ollama should not be removed just because llama.cpp becomes the primary path. It is the escape hatch for unsupported platforms, non-NVIDIA systems, users with existing Ollama libraries, and users who do not want MagicHandy to manage model files.

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