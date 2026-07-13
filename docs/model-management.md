# Local Model Management

## Purpose

MagicHandy manages local LLM setup deliberately. Managed llama.cpp is the
quality-first path; Ollama remains a first-class external provider. The model
manager gives llama.cpp a durable inventory without making model downloads or
runtime discovery part of startup.

The inventory, local imports, and app-owned llama.cpp source-build lifecycle are
implemented. Curated model downloads and hardware-fit recommendations remain
release work.

## Ownership Boundaries

- `internal/llm.ModelManager` owns model records, managed copies, and import
  jobs.
- SQLite schema v9 owns model metadata in `llm_models`.
- Model files live under the app data directory, outside SQLite.
- `ManagedLlamaRuntimeManager` owns explicit build/cancel state and activates
  only validated app-owned runtime manifests.
- The managed llama.cpp provider owns only its runner process. The backend
  resolves the active runner and selected model ID to app-owned paths, then
  exposes that ID under a stable `--alias`. Paths are not settings.
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
  runtimes/
    llama.cpp/
      active.json               constrained active-runtime manifest
      .tools/                   embedded build helper materialized at use
      installs/
        b9966-<backend>-c749cb0/
          runtime.json
          LICENSE-llama.cpp
          bin/                  llama-server plus required shared libraries
```

Imports write to `downloads` on the same filesystem, flush the file, verify it,
then rename it into the model store. Startup removes only model-import partials
older than 24 hours, so another process starting on the same data directory
does not immediately unlink an active copy.

## Managed llama.cpp Runtime Build

Managed mode never asks for `llama-server` or GGUF paths. MagicHandy pins
llama.cpp release `b9966` at commit
`c749cb041706647f460bb918cccc9d91995205ab` and embeds the PowerShell build
helper in the Go binary. **Build runtime** is an explicit, controller-gated
action. The same helper is called by `install.ps1` when the user accepts its
managed-runtime prompt. Direct helper and in-app builds still require the tools
to exist; the source installer detects, installs, and verifies missing Git,
CMake, Visual Studio Desktop C++/Windows SDK, and selected CUDA dependencies
before invoking it.

The helper:

1. requires Windows/amd64 plus Git, CMake, and Visual Studio C++ Build Tools;
2. chooses CUDA in `auto` mode only when NVIDIA tooling and `nvcc` are present,
   otherwise CPU;
3. fetches the pinned tag with Git long-path support and verifies the exact
   commit;
4. builds only the local `llama-server` target, with curl, HTTPS, and embedded
   llama.cpp UI assets disabled;
5. probes the resulting executable for commit `c749cb0`;
6. copies the complete binary/DLL set and MIT license into a versioned staging
   directory; and
7. atomically writes `active.json` only after the install is valid.

Build source and intermediates use a job-specific temporary directory beneath
the runtime root and are removed after success or failure. Runtime inspection
does no network I/O and starts no process. The managed server itself launches
with `--offline --no-ui`, binds to MagicHandy's fixed loopback endpoint, and
loads only the backend-resolved managed model. An incomplete or mismatched
app-owned install is replaced on retry; users are not asked to repair runtime
directories by hand.

The app exposes build state, bounded output, cancellation, installed version,
backend, and current/outdated/invalid state. Cancellation terminates the
PowerShell build process tree on Windows; app shutdown cancels and waits for an
active build.

The interactive installer explains the tradeoff before building. Managed
llama.cpp gives MagicHandy direct version, startup, loading, and diagnostics
control. Users with an existing Ollama setup can answer **No** (or pass
`-SkipLlamaBuild`) to avoid the extra runtime. Ollama models remain in place
unless the user separately chooses **Import from Ollama**, which intentionally
creates a managed copy.

Successful source installs persist only these non-secret provisioning choices
under LocalAppData. `update.ps1` displays and preserves them by default, or can
revisit the managed-build/backend/Ollama decisions before rebuilding. Disabling
a choice never silently deletes an existing runtime or model library.

## Standalone GGUF Import

The user provides a local file path and optional display name. MagicHandy:

1. requires a regular file with the `GGUF` magic header;
2. starts an asynchronous, cancellable copy;
3. computes SHA-256 while copying;
4. commits the file and metadata atomically;
5. deduplicates the inventory by SHA-256; and
6. leaves the source file unchanged.

The selected model cannot be removed. Selection stores only the managed model
ID, then follows the normal Save settings flow. At provider construction the
backend resolves that ID to a ready inventory record; a missing, changed, or
unknown record is a visible unavailable state, never a fallback path.

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
- app-owned runtime version/backend/build state with CPU/CUDA/auto build and
  cancel controls in managed mode;
- no executable or model path inputs in managed mode;
- server-reported external llama.cpp and Ollama model rows with matching
  Use/Selected behavior, while retaining free-form external-provider inputs;
- managed model rows with source, size, quantization, state, Use, and guarded
  Remove actions;
- standalone GGUF and Ollama import disclosures;
- bounded Ollama candidate rows with filtering and compatibility reasons; and
- copy progress, failure text, and cancellation.

Generation controls stay deliberately small and provider-aware:

- **Maximum output** defaults to 256 tokens and applies to both passes.
  llama.cpp receives `max_tokens`; Ollama receives `options.num_predict`.
  Provider `length` completion reasons are handled as truncation rather than a
  successful empty response. The UI exposes reviewed 128/256/512/1024 choices.
- **Thinking / reasoning** supports `off` (the default) and `auto`. `off` sends
  `chat_template_kwargs.enable_thinking=false` to the pinned llama.cpp server
  and top-level `think=false` to Ollama. Some templates/models can ignore or
  reject this override. `auto` retains provider/model behavior; the current
  pinned managed llama.cpp bounds hidden reasoning to half the selected
  total token budget so compact JSON has room. Outdated managed and external
  providers retain their native behavior and rely on the repair fallback.
- Repair temperature `0` is serialized explicitly, repair always requests
  reasoning off where supported, and the original conversation remains in
  repair context. Prompt
  examples are parser-valid and an immutable final guard makes reply-only JSON
  the uncertainty fallback, following the strongest small-model lesson from the
  STGPT-RV prompt inventory. Managed llama.cpp remembers
  a successful load and skips redundant `/health` and `/v1/models` probes on
  subsequent warm chat/repair calls; explicit status and cold load still probe.

These are latency controls, not a measured speed claim. Keep output quality,
malformed/repair rate, cold load, prompt evaluation, hidden reasoning, visible
generation, and total time separate when comparing models.

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
- RAM/VRAM and GPU-fit recommendations;
- context-window and JSON-compliance scoring;
- resumable downloads and persisted cross-restart import jobs; and
- split-GGUF and auxiliary projector/adapter launch support if the managed
  runner contract grows to support them safely.

## Diagnostics And Privacy

Diagnostics may include provider type, selected model ID, managed metadata,
runner version/backend/status/errors, build state/output tail, import state,
and load timings. They must not include
model bytes, full private chat logs, prompt bodies, connection keys, or API
keys. Local filesystem paths are operational metadata, not credentials, but
exports should still avoid including unrelated paths.
