# ADR 0001: Go-First Core

## Status

Accepted for the rewrite plan.

## Context

MagicHandy is a ground-up rewrite of StrokeGPT-ReVibed. The current app has accumulated a large Python backend, a browser UI, optional Python ML dependencies, and a motion stack with many hard-won real-device constraints. The rewrite is being pursued for maintainability, cleaner long-running architecture, future binary releases, and lower non-ML baseline overhead.

The rewrite is not based on the claim that Go will make Handy cloud REST latency disappear, reduce Ollama model memory, reduce CUDA model memory, or automatically make motion smooth. Motion quality must come from better motion semantics, retargeting, transport scheduling, diagnostics, and real-device validation.

## Decision

Use Go for the core MagicHandy application.

The Go core owns:

- app process lifecycle
- HTTP API and local UI serving
- settings and migrations
- chat orchestration and Ollama transport
- deterministic motion engine
- Handy transport scheduling and diagnostics
- mode planners
- trace capture and diagnostics bundles
- optional worker lifecycle management

The Go core does not require Python to start, serve the UI, load settings, control motion, or run diagnostics.

Python remains acceptable for optional ML-heavy workers, especially Chatterbox, faster-whisper, Parakeet, Torch, CUDA, or other model runtimes that are impractical to port early.

## Why Go

Go is selected for the core because it gives a pragmatic balance of:

- simple static binary distribution
- reliable cross-platform builds, especially Windows
- low baseline memory compared with a Python web stack
- straightforward HTTP, SSE, and WebSocket servers
- goroutines and channels for long-running transport/status loops
- built-in race detector for concurrent code
- fast compile/test cycles
- simpler operational story for non-developer users

## Why Not Rust-First For The Whole App

Rust is attractive for motion math and transport correctness, but it is a larger implementation burden for the whole app. The project benefits more from shipping a maintainable core than from maximizing low-level performance. Rust can be reconsidered later for a motion library if Go code proves insufficient in a specific measured area.

## What Go Is Expected To Improve

- Maintainability of the app core.
- Packaging and binary release experience.
- Baseline memory and startup cost of the non-ML app.
- Concurrency structure around motion, transport, status, and diagnostics.
- Separation of optional ML workers from the core runtime.
- Testability of transport and motion contracts.

## What Go Is Not Expected To Improve By Itself

- Handy cloud REST round-trip latency.
- Remote API behavior or device firmware behavior.
- Ollama model memory usage.
- CUDA context/model memory usage.
- Chatterbox/faster-whisper/Parakeet dependency complexity when those workers are installed.
- Motion smoothness without a better retargeting and transport model.

## Consequences

Positive:

- MagicHandy can become a normal downloadable app instead of a Python environment project.
- The core can run even when optional voice dependencies are absent or broken.
- Long-running loops can be modeled explicitly and race-tested.

Negative:

- The rewrite starts without the current Python test corpus.
- Python and Go codebases may drift while both exist.
- Existing behavior must be intentionally re-specified or it will be lost.
- Optional Python workers add IPC and lifecycle complexity.

## Implementation Guidance

- Preserve StrokeGPT-ReVibed motion/transport invariants as executable tests early.
- Keep the core sidecar-compatible so the current Python app can drive it during validation if that reduces risk.
- Do not port legacy architecture blindly.
- Treat real-device validation as a required milestone before broad feature parity.
