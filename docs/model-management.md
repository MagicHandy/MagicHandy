# Local Model Management

## Purpose

MagicHandy manages local LLM setup deliberately. Managed llama.cpp is the
quality-first path; Ollama remains a first-class external provider. The model
manager gives llama.cpp a durable inventory without making model downloads or
runtime discovery part of startup.

The inventory and local import foundation is implemented. Curated downloads,
runner provisioning, and hardware-fit recommendations remain release work.

## Ownership Boundaries

- `internal/llm.ModelManager` owns model records, managed copies, and import
  jobs.
- SQLite schema v9 owns model metadata in `llm_models`.
- Model files live under the app data directory, outside SQLite.
- The managed llama.cpp provider owns only its runner process. It receives the
  selected model path from settings and exposes that model under a stable
  `--alias`.
- Ollama owns its daemon and library. MagicHandy may read its manifests and
  blobs during an explicit import, but never modifies or deletes them.
- Provider status and model listing never download or load a model.

## Model Records

Each managed record includes:

- stable ID and display name
- provider compatibility (currently `llama_cpp`)
- source (`gguf` or `ollama`) and source model name
- format, family, parameter size, and quantization when known
- file size and SHA-256
- managed file path
- a short license label when present in the source manifest
- import/update timestamps
- computed file state (`ready`, `missing`, or `changed`)

The inventory is not a model-quality catalog. JSON reliability, prompt fit,
license URLs, context limits, RAM/VRAM guidance, and curated source URLs belong
to the later curated catalog.

## Storage Layout

```text
data/
  magichandy.db                 llm_models metadata (schema v9)
  models/
    gguf/
      <model-id>/
        model.gguf
        metadata.json
  downloads/
    model-import-<job-id>.partial
```

Imports write to `downloads` on the same filesystem, flush the file, verify it,
then rename it into the model store. Startup removes only model-import partials
older than 24 hours, so another process starting on the same data directory
does not immediately unlink an active copy.

## Standalone GGUF Import

The user provides a local file path and optional display name. MagicHandy:

1. requires a regular file with the `GGUF` magic header;
2. starts an asynchronous, cancellable copy;
3. computes SHA-256 while copying;
4. commits the file and metadata atomically;
5. deduplicates the inventory by SHA-256; and
6. leaves the source file unchanged.

The selected model cannot be removed. Selection updates the managed llama.cpp
model ID and path together, then follows the normal Save settings flow.

## Ollama Import

The Model screen has an **Import from Ollama** disclosure. The library path is
editable and persisted as `llm.ollama_models_path`; an empty value uses:

1. `OLLAMA_MODELS`, when set;
2. `~/.ollama/models` on Windows/macOS and when present on Linux; or
3. `/usr/share/ollama/.ollama/models` on Linux.

The scanner also accepts the parent `.ollama` directory and resolves its
`models` child. This matches Ollama's documented storage and
[`OLLAMA_MODELS`](https://docs.ollama.com/faq) behavior.

Ollama manifests reference content-addressed blobs. MagicHandy accepts a
candidate only when:

- the manifest is bounded JSON schema 2;
- exactly one `application/vnd.ollama.image.model` layer exists;
- no separate adapter or projector layer is required;
- the blob exists, has the manifest size, and starts with `GGUF`; and
- the config reports GGUF when it reports a format.

Multi-layer/split models and models requiring auxiliary projector/adapter
arguments are shown with an incompatibility reason. The managed provider does
not silently drop those layers.

Import re-scans the candidate, copies its model blob, computes SHA-256, and
requires an exact match with the manifest digest before commit. The Ollama
source remains untouched. This intentionally duplicates disk use: directly
pointing llama.cpp at a content-addressed Ollama blob would let an Ollama prune
break MagicHandy's selected model and would not give MagicHandy ownership of
the file lifecycle.

## Ollama Provider Model List

The external provider list comes from `GET /api/tags` through a five-second,
non-loading request. It is independent of the selected model, so setup can list
available Ollama models even when the current model name is invalid. The list
includes name, size, format, family, parameter size, and quantization when the
daemon reports them.

## UI Contract

Settings > Model shows:

- saved runtime health and the provider's last status message;
- provider-scoped fields only;
- a model combobox backed by managed or daemon-reported models while retaining
  a free-form external-provider escape hatch;
- managed model rows with source, size, quantization, state, Use, and guarded
  Remove actions;
- standalone GGUF and Ollama import disclosures;
- bounded Ollama candidate rows with filtering and compatibility reasons; and
- copy progress, failure text, and cancellation.

Runtime Load/Unload actions are available only for managed llama.cpp. They are
controller-gated in both UI and HTTP and remain disabled while the settings
form differs from the saved runtime configuration.

## Limits And Remaining Work

Implemented limits:

- at most 2,000 Ollama manifests per scan;
- at most 1 MiB per manifest, 256 KiB per config/license blob;
- model files bounded to 1 TiB as a defensive ceiling;
- at most two concurrent copies and 64 recent in-memory import jobs;
- no automatic downloads, startup scans, or source-library writes; and
- recent import progress is process-local; completed model records are durable.

Still planned:

- checksum-pinned curated model downloads with license/source metadata;
- llama.cpp runner provisioning and version/backend inventory;
- RAM/VRAM and GPU-fit recommendations;
- context-window and JSON-compliance scoring;
- resumable downloads and persisted cross-restart import jobs; and
- split-GGUF and auxiliary projector/adapter launch support if the managed
  runner contract grows to support them safely.

## Diagnostics And Privacy

Diagnostics may include provider type, selected model ID, managed metadata,
runner status/errors, import state, and load timings. They must not include
model bytes, full private chat logs, prompt bodies, connection keys, or API
keys. Local filesystem paths are operational metadata, not credentials, but
exports should still avoid including unrelated paths.
