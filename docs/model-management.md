# Local Model Management

## Purpose

MagicHandy manages local LLM setup deliberately. Managed llama.cpp is the
quality-first path; Ollama remains a first-class external provider. The model
manager gives llama.cpp a durable inventory without making model downloads or
runtime discovery part of startup.

The inventory, local imports, app-owned llama.cpp release lifecycle, and
curated downloads with GPU-fit ratings are implemented. The curated catalog and
its trust model are recorded in
[ADR 0034](decisions/0034-curated-model-downloads.md).

## Ownership Boundaries

- `internal/llm.ModelManager` owns model records, managed copies, and import
  jobs.
- SQLite schema v9 owns model metadata in `llm_models`.
- Model files live under the app data directory, outside SQLite.
- `ManagedLlamaRuntimeManager` owns explicit install/cancel state and activates
  only validated app-owned runtime manifests.
- The managed llama.cpp provider owns only its runner process. The backend
  resolves the active runner and selected model ID to app-owned paths, then
  exposes that ID under a stable `--alias`. Paths are not settings.
- Ollama owns its daemon and library. MagicHandy may read its manifests and
  blobs during an explicit import, but never modifies or deletes them.
- Provider status and model listing never download or load a model.
- Curated downloads start only from an explicit Download action or a reviewed
  setup plan, and only for an ID from the built-in catalog.

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

The inventory is not a model-quality catalog. JSON reliability and prompt fit
live in [model-suitability.md](model-suitability.md); license links, source
pages, measured graphics memory and pinned digests live in the curated catalog.

## Curated Downloads

`internal/llm/model_catalog.go` ships three tested Gemma 4 builds, in preference
order: the 12B heretic default (a content-addressed Ollama registry blob), a
lighter 12B QAT build and an E4B QAT build for 6 to 8 GB cards (Hugging Face
files pinned to one repository commit). `GET /api/llm/catalog` returns each
entry with a `fit` for the detected GPU (`recommended` for the first entry the
card holds, `supported`, `below_minimum`, `unknown_vram`, `no_gpu`), the managed
model ID when the digest is already in the store, and the size of any resumable
partial.
`POST /api/llm/imports/catalog` takes a catalog ID, checks free space for the
remaining bytes plus 512 MiB, and starts an import job with status
`downloading`. The job resumes from `downloads/catalog-<sha256>.partial`, hashes
the saved prefix and every new byte, rejects any other size or digest, validates
the GGUF, and commits through the normal store path with source `ollama` for
registry blobs or `gguf` for Hugging Face files.
Cancellation and transport failures keep the partial; a checksum failure
discards it. Startup removes only partials whose digest the catalog no longer
lists.

## Chat Template Fixes

The metadata scan that rejects unsupported GGUFs also records the architecture
and the SHA-256 of the embedded chat template. Known defective templates map to
a fix (`knownTemplateFixes` in `internal/llm/model_fixes.go`). Today the only fix
is `gemma4-close-thinking`: Google's canonical Gemma 4 template with the thought
channel closed when thinking is off. Without it, fine-tuned E4B builds reason
silently on every turn. A model with a known template runs with
`--chat-template-file` pointing at `models/templates/<fix>.jinja`, which is
written from the binary. A Gemma 4 model whose template has the same defect but
was never measured is only offered the fix (`template_fix_offer`), and the user
turns it on per model from the model test. The choice lives in `app_kv` as
`llm.template_fix_overrides`. See
[ADR 0035](decisions/0035-model-test-and-template-fixes.md).

## Model Test

Settings > Chat > Model > **Test model** opens a window that runs five scripted
turns through the real chat service against the selected model. It reports
load time, GPU layer placement, the template fix state, runtime currency,
hidden reasoning, reply-format repairs, truncation, speed, reply length and
refusals. The user starts it; it never runs automatically and never commands
motion. API: `GET`/`POST`/`DELETE /api/llm/model-check` (host-only), and
`POST /api/llm/models/{id}/template-fix` with `{"enabled": bool}`.

## Storage Layout

```text
data/
  magichandy.db                 llm_models metadata (schema v9)
  models/
    gguf/
      <model-id>/
        model.gguf
        metadata.json
    templates/
      gemma4-close-thinking.jinja   written from the binary when a fix applies
  downloads/
    model-import-<job-id>.partial
  runtimes/
    llama.cpp/
      active.json               constrained active-runtime manifest
      .tools/                   embedded installer and upstream license
      downloads/                resumable archive partials during installation
      installs/
        b11149-<backend>-d2e5458/
          runtime.json
          provenance.json
          LICENSE-llama.cpp
          bin/                  llama-server plus required shared libraries
```

Imports write to `downloads` on the same filesystem, flush the file, verify it,
then rename it into the model store. Startup removes only model-import partials
older than 24 hours, so another process starting on the same data directory
does not immediately unlink an active copy.

## Managed llama.cpp Runtime Installation

Managed mode never asks for `llama-server` or GGUF paths. MagicHandy pins
llama.cpp release `b11149` (the build for upstream `v0.5.0`) at commit
`d2e54583c7452353eb35d40431281f6ee984332f` and embeds the PowerShell installer
plus upstream MIT license in the Go binary. **Install runtime** is an explicit,
controller-gated action. The same helper is called by `install.ps1` when the
user accepts its managed-runtime prompt. No Git, CMake, C++ compiler, MSYS2, or
CUDA Toolkit is required.

The helper:

1. requires Windows/amd64 and chooses CUDA in `auto` mode when a working NVIDIA
   driver and GPU are present, otherwise CPU;
2. downloads official `b11149` Windows CPU or CUDA 12.4 runner/runtime archives
   over HTTPS with retry and partial resume;
3. verifies exact archive sizes and SHA-256 digests before extraction;
4. rejects rooted, traversing, duplicate, or symbolic-link archive entries;
5. probes the resulting executable for commit `d2e5458` and, for CUDA, requires
   a detected CUDA device;
6. copies the complete binary/DLL set and MIT license into a versioned staging
   directory and records archive provenance; and
7. atomically writes `active.json` only after the install is valid.

When a release moves the pin, an installed older runtime reports `outdated`.
With `ui.runtime_update_mode` set to `automatic` (the default), startup installs
the pinned runtime with the backend already in use and loads the model when it
finishes. With `manual`, the notification center and Settings > General >
Updates offer **Update now**. See
[ADR 0036](decisions/0036-managed-runtime-updates.md).

Extraction intermediates use a job-specific temporary directory beneath the
runtime root and are removed after success or failure. Verified archives are
removed after activation; interrupted partial downloads remain resumable. Runtime inspection
does no network I/O and starts no process. The managed server itself launches
with `--offline --no-ui`, one generation slot (`--parallel 1`), and the saved
context size. The reviewed choices are 16,384, 32,768 (default), 65,536, and
131,072 tokens. It binds to MagicHandy's fixed loopback endpoint and loads only
the backend-resolved managed model. A bounded explicit context avoids allocating
a model-advertised 262k window for measured 7k-12k-token requests, while one slot
matches the app's one interactive stream and cancel-before-yield Autopilot
behavior. Larger contexts consume more RAM and VRAM, and a value below the
prompt length cannot fit the request. GPU offload, Flash Attention, batching,
and ordinary prompt caching retain the pinned runner's automatic defaults so
CPU builds and competing voice workloads can still fit. An incomplete or
mismatched app-owned install is replaced on retry; users are not asked to repair
runtime directories by hand.

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

1. requires a regular file with a supported GGUF header and bounded metadata;
2. rejects split shards and embedded audio, vision, or projector components that
   the managed text-only runner cannot load;
3. starts an asynchronous, cancellable copy;
4. computes SHA-256 while copying;
5. commits the file and metadata atomically;
6. deduplicates the inventory by SHA-256; and
7. leaves the source file unchanged.

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
- the blob exists, has the manifest size, and has supported bounded GGUF
  metadata with no split-shard declaration or embedded audio, vision, or
  projector component; and
- the config reports GGUF when it reports a format.

Multi-layer/split models and models requiring auxiliary audio, vision,
projector, or adapter handling are shown with an incompatibility reason. The
managed provider does not silently drop those components. Existing managed
copies are classified the same way during inventory reads, with the result
cached until the file size or modification time changes.

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

- **Context size** is a managed llama.cpp process-allocation setting, not a
  request option. It is durable, defaults to 32,768, and exposes only the four
  backend-reviewed values above. Saving a changed value unloads a stale managed
  process; the next Load or chat starts it with the new `--ctx-size`. External
  llama.cpp and Ollama ignore this setting, including for provider cache
  identity, because those runtimes own their context configuration.
- **Cold-load readiness** treats llama.cpp's bounded HTTP 503 `Loading model`
  response as a running/loading state rather than a runtime failure. Managed
  startup waits up to 90 seconds for slower first reads, and the Model screen
  polls that state until it becomes ready. Because the pinned app-owned health
  endpoint reserves 503 for cold loading, a live managed process also treats a
  bodyless or malformed 503 as loading; external llama.cpp servers retain the
  stricter body check.
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
  repair context. During an interactive repair, an already-visible first-pass
  reply remains stable until the backend sends the authoritative final repaired
  message; when no first-pass reply was parseable, the repair draft may stream.
  Prompt
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
- aggregate request budgeting plus context-window and JSON-compliance scoring;
- resumable downloads and persisted cross-restart import jobs; and
- split-GGUF and auxiliary projector/adapter launch support if the managed
  runner contract grows to support them safely.

## Diagnostics And Privacy

Diagnostics may include provider type, selected model ID, saved managed context
size, managed metadata, runner version/backend/status/errors, build state/output
tail, import state, and load timings. They must not include
model bytes, full private chat logs, prompt bodies, connection keys, or API
keys. Local filesystem paths are operational metadata, not credentials, but
exports should still avoid including unrelated paths.
