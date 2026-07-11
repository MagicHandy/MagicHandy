# Installation Automation — Parity Plan

## Goal

A non-technical user should be able to get MagicHandy running — app, LLM, model,
and device — with as little friction as StrokeGPT-ReVibed offered, ideally less.
This doc compares the two, records what already exists, and lays out the roadmap
to parity. It is the reference for the interactive installer (`install.ps1`) and
the eventual in-app setup wizard — whose shape is now decided:
[docs/gui-installer.md](gui-installer.md) records the evaluation (2026-07-11)
that the GUI installer is the app's own first-run wizard behind a thin Inno
Setup shell, with the Windows setup binary and wizard slices scheduled in
Phase 16.

A structural advantage worth stating up front: MagicHandy's core is a single
pure-Go binary with **no Python, no venv, no pip, and no torch/CUDA in the core**.
The hardest, most failure-prone part of the old setup — building a Python ML
environment with matching CUDA/torch wheels — simply does not exist here. CUDA
matters only for the app-owned external `llama-server` process, never for the
MagicHandy Go core itself.

## What StrokeGPT-ReVibed automated (the parity target)

- A Python virtual environment plus dependency install (including heavy ML deps).
- GPU/CUDA handling: detecting the GPU and installing matching torch/CUDA wheels,
  with a CPU fallback.
- Model selection and download — LLM models plus voice (ASR/TTS) models — via a
  manager rather than manual file wrangling.
- Start/Stop convenience scripts.
- A first-run configuration path so the user did not start from a blank state.
- (For LAN/mobile use) a local certificate/HTTPS helper.

## What MagicHandy has today

- **Build/run from source:** `go run ./cmd/magichandy` (or `go build`). No venv,
  no pip.
- **Interactive installer (`install.ps1`, this repo):** checks for Go and offers
  to install it (winget), builds the binary, sets up a data folder, detects an
  NVIDIA GPU, and offers a pinned managed llama.cpp source build with CPU/CUDA
  selection. It explains why direct llama.cpp control helps and lets existing
  Ollama users decline the build to save space (`-SkipLlamaBuild` supports the
  same choice in unattended runs). It also offers an optional local Parakeet
  ASR setup. The Parakeet path downloads a pinned CPU runner and model only
  after consent, shows size and license, verifies SHA-256, builds the worker,
  and leaves voice disabled. The installer can also write a
  `Start-MagicHandy.ps1` launcher and open the app.
- **Local model manager:** Settings > Model lists runtime/daemon models and
  SQLite-backed managed GGUF copies. Users can import a standalone GGUF or scan
  a configurable Ollama library path and copy a compatible model with
  SHA-256-verified progress. Managed llama.cpp can also build/switch its pinned
  app-owned runtime from this screen; the core never builds or downloads a
  runtime/model on startup.
- **Local data:** settings, memory, prompt sets, chat history, patterns,
  programs, preference feedback, and model metadata in a local SQLite DB; the
  Handy connection key is stored locally and never echoed back.

## Gaps vs the target

1. No packaged release yet — install still requires Go to build (Phase 16).
2. No in-app curated model catalog or guided network download yet. Local GGUF
   and compatible Ollama-library imports are implemented with copy progress.
3. GPU handling chooses CPU/CUDA from available build tooling, but does not yet
   recommend a model from detected VRAM or install the CUDA toolkit itself.
4. No in-app first-run setup wizard (the installer script is the current
   stand-in).
5. Voice setup is partial: Parakeet ASR has an installer path, but in-app
   provider provisioning, microphone UI, local cloning TTS, and any LAN/HTTPS
   story remain open (Phase 13; R17/R18).

## Roadmap to parity

Ordered roughly by leverage. Each step keeps the cross-cutting rules below.

1. **Bootstrap installer — done** (`install.ps1`): Go, build, data folder, LLM
   guidance, launch. The entry point until releases exist.
2. **Model-manager foundation (done).** Durable model inventory, provider model
   list, standalone GGUF import, configurable Ollama-path scan/import,
   checksum verification, cancellation, selection, and guarded removal.
3. **Packaged releases (Phase 16).** Windows portable zip / signed build so the
   installer can *download a prebuilt binary* instead of building from source —
   no Go required for end users. Linux/macOS artifacts best-effort.
4. **Managed llama.cpp runner provisioning (done for source installs).** The
   installer and Model UI invoke one embedded helper pinned to `b9966` /
   `c749cb0`, verify the checkout and executable, build CPU or CUDA, install the
   complete runtime atomically, and activate a constrained app-data manifest.
   No user path setting remains. Phase 16 packaging can ship prebuilt outputs so
   release users do not need Git/CMake/Visual Studio.
5. **Curated model catalog + guided download.** A small, opinionated list of
   recommended GGUF models with visible size, license, checksum, and rough VRAM
   fit; one-click download with progress; and "import a local GGUF" without a
   download. Startup and status checks never trigger a download (guardrail).
6. **GPU/VRAM aware recommendations.** Use `nvidia-smi` (and VRAM) to recommend a
   runner build and a model size that fits, with an honest CPU fallback and a
   note on driver/CUDA-toolkit expectations. Because CUDA lives only in the
   external runner, this stays far simpler than the old torch/CUDA install.
7. **In-app first-run setup wizard.** The eventual best UX and the real parity
   milestone for non-technical users: connect the Handy (enter key, test),
   choose a provider, pick/download a model, and confirm — all in the browser UI,
   superseding the script for most people. Designed in
   [docs/gui-installer.md](gui-installer.md) (Phase 16 slices 16.1-16.3),
   including voice provisioning moved behind API endpoints and the
   StrokeGPT-ReVibed porting step over the Phase 15 importer.
8. **Voice setup (Phase 13).** Parakeet ASR's explicit installer path is the
   first slice. Add an in-app guided profile, local TTS provisioning, and
   microphone calibration only after real-device and microphone evidence;
   providers stay optional and off the core install path (ADR 0007).
9. **Cross-platform + LAN.** Linux/macOS install scripts; decide the LAN/mobile
   HTTPS story explicitly before promising phone use (risk R18).

## Cross-cutting rules

These hold for every step above (from `docs/goals-and-guardrails.md` and
`AGENTS.md`):

- **Downloads are explicit user actions** with visible size, license, checksum,
  and disk-use; verify before install and move files atomically. Startup and
  status checks must never kick off a multi-GB download.
- **The core stays pure-Go.** CUDA/torch and any native ML live in the external
  runner or a worker process, never in the MagicHandy binary.
- **Secrets never touch logs or the catalog** — the connection key and any API
  keys stay local and redacted.
- **Provider choice is visible state**, not a silent mid-session switch, and the
  app runs (chat aside) even with no model configured.

## Parity checklist

| StrokeGPT-ReVibed setup capability | MagicHandy status | Where |
| --- | --- | --- |
| One-command environment setup | Partial — `install.ps1` builds from source | this repo |
| No Python/venv/torch to install | **Better** — pure-Go core, none needed | by design |
| Prebuilt one-click download | Planned | Phase 16 |
| LLM runner provisioning (CUDA/CPU) | **Implemented for source installs** | installer + Settings > Model |
| Model selection + local/Ollama import UI | **Implemented** | Settings > Model |
| Curated model download UI | Planned | catalog + Settings > Model |
| GPU/VRAM-aware recommendations | Detection + advice today | install.ps1 GPU detection |
| Start/Stop convenience | `Start-MagicHandy.ps1` (opt-in) | install.ps1 |
| First-run setup wizard | Planned (script is the stand-in) | step 7 |
| Voice model setup | Partial - opt-in Parakeet ASR installer path | Phase 13.4 |
| LAN/mobile HTTPS helper | Undecided (scope in R18) | step 9 |

## Related docs

- `install.ps1` — the interactive installer.
- `IMPLEMENTATION_PLAN.md` — Phase 16 (packaging), Phases 12-13 (voice).
- `docs/model-management.md` — model catalog and llama.cpp runner strategy.
- `docs/risk-register.md` — R13 (runner/model management), R18 (LAN/HTTPS).
- `docs/goals-and-guardrails.md` — the download/pure-Go/secret rules above.
