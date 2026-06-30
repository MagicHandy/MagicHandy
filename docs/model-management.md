# Local Model Management

## Purpose

MagicHandy will manage local LLMs deliberately instead of hiding model setup behind startup code. The quality-first path is managed llama.cpp on Windows/NVIDIA. Ollama remains the secondary cross-platform path for externally managed models.

This document defines how model files, downloads, imports, runtime selection, and user-facing status should work.

## Provider Priority

Default product direction:

1. llama.cpp, when running on a supported Windows/NVIDIA setup with an installed compatible GGUF model.
2. Ollama, when selected by the user or when llama.cpp is unavailable/unsupported.
3. No local LLM, with clear setup actions, when neither provider is ready.

Provider fallback must be visible. The app should not silently switch a running chat session from llama.cpp to Ollama or from Ollama to llama.cpp because that makes response quality, prompt behavior, and diagnostics hard to reason about.

## Model Records

A model record should include:

- stable ID
- display name
- provider compatibility (`llama.cpp`, `ollama`, or both when applicable)
- source URL or repository
- filename for GGUF models
- quantization
- file size
- checksum
- license and license URL
- context window
- expected system RAM and VRAM range
- recommended runner/backend profile
- JSON reliability notes
- prompt-fit notes for MagicHandy chat and motion control
- installed path or external model name
- installed/downloaded/failed state

Keep the curated catalog small at first. A shorter list of models that work well is better than a broad list that produces malformed JSON or poor motion intent.

## Storage Layout

Recommended data layout:

```text
data/
  models/
    gguf/
      <model-id>/
        model.gguf
        metadata.json
  downloads/
    <download-id>.partial
  model-index.json
  model-install-state.json
```

The exact app-data root is decided by the config package, but model storage should stay separate from settings and logs so disk cleanup is understandable.

## Download Flow

Downloads are explicit user actions.

Required flow:

1. User selects a model.
2. UI shows size, license, expected hardware fit, and disk impact.
3. User confirms download.
4. App downloads to a temporary/incomplete file.
5. App verifies checksum.
6. App moves the verified file atomically into the model store.
7. App records install metadata.
8. User explicitly loads the model or enables auto-load for future startup.

Startup, provider status checks, diagnostics, and first chat must not trigger large model downloads.

## Import Flow

Users can import an existing local GGUF file.

Import should:

- copy or register the file according to a clear user choice
- calculate checksum
- infer metadata where possible
- require the user to fill missing license/source data if the model will be exported or shared in diagnostics
- validate that the selected llama.cpp runner can at least attempt to load the file

## llama.cpp Runner Handling

The llama.cpp provider should treat the runner as managed runtime state:

- runner path
- runner version
- acceleration profile
- launch arguments
- selected model path
- localhost port
- process ID
- health status
- last stderr excerpt
- last load error

The Go core starts and stops the runner process. It does not link libllama and does not require CGo.

The first supported target is Windows/amd64 with NVIDIA acceleration. Other llama.cpp backends can be added later behind the same provider contract.

## Ollama Handling

Ollama is secondary and externally managed.

MagicHandy should:

- connect to the configured Ollama host
- list installed models
- allow selecting an Ollama model
- report daemon/model errors clearly
- avoid trying to manage Ollama's internal model files directly
- use the same chat, prompt, repair, malformed-response, and motion pathways as llama.cpp

## UI Requirements

The Model section should show:

- active provider
- active model
- provider health
- installed llama.cpp GGUF models
- curated downloadable llama.cpp models
- import GGUF action
- download progress and disk usage
- load/unload controls
- Ollama connection/model status
- hardware-fit warnings
- last runner or provider error
- advanced runner args behind a disclosure control

Quality warnings should be direct. If a model is known to produce weak JSON or poor motion-control compliance, say so before the user chooses it.

## Diagnostics

Diagnostics may include:

- provider type and version
- runner version
- acceleration profile
- selected model ID
- model metadata
- load/unload timings
- prompt/response token counts when available
- structured errors

Diagnostics must not include full private chat logs, secrets, large prompt bodies, or model files.