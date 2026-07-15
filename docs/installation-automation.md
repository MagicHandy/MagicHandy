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
- **Interactive installer (`install.ps1`, this repo):** can bootstrap a clean
  64-bit Windows machine. It repairs/installs WinGet through Microsoft's
  supported PowerShell path when needed, then installs and verifies Go. A
  selected managed llama.cpp source build additionally provisions Git, CMake,
  the Visual Studio Desktop C++ workload/Windows SDK, CUDA when selected, and
  LLVM/libclang and pinned Rust 1.94.0 through Rustup for the coupled NeuTTS
  build. It builds the pinned `stream_pcm` runner and first-party ONNX reference
  encoder, converts a verified NeuCodec checkpoint, and installs the verified
  Air Q4 and DistillNeuCodec assets. Choosing Ollama avoids both managed source
  builds; the installer can provision Ollama too. It builds the core and all
  three first-party Go voice adapters. Optional Parakeet assets remain consented,
  size/license-visible, and SHA-256 verified, and voice remains disabled. The
  installer can write a `Start-MagicHandy.ps1` launcher and open the app.
- **State-aware source updater (`update.ps1`):** atomically reads the non-secret
  install choices stored under LocalAppData, shows them, asks whether to revise
  them, refuses a dirty worktree, resolves an explicit safe fast-forward target,
  and rebuilds through the same provisioning implementation. Live feature
  branches follow their upstream; merged features with a deleted upstream may
  advance from `origin/main` only after an ancestry check. Provider credentials
  and the Handy connection key never enter installer state. Rebuilds first send
  Emergency Stop and terminate only this checkout's app process tree; binaries
  are staged before replacement, and browser launch waits for the rebuilt
  process to own the configured port and answer `/api/state`.
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

1. No packaged release yet. Source setup no longer requires Go or the compiler
   to be preinstalled, but it installs a multi-GB developer toolchain when a
   managed source build is selected. Phase 16 still owns a prebuilt path that
   does not install those tools at all.
2. No in-app curated model catalog or guided network download yet. Local GGUF
   and compatible Ollama-library imports are implemented with copy progress.
3. GPU handling can install the CUDA Toolkit when CUDA is explicitly selected,
   but does not yet recommend a model from detected VRAM.
4. No in-app first-run setup wizard (the installer script is the current
   stand-in).
5. Voice setup is partial: provider adapters, provider-scoped settings,
   continuous hands-free and hold-to-talk browser capture, app-managed Parakeet
   and NeuTTS installer paths, and guarded local Windows path browsing exist.
   App-managed assets are discovered separately from custom overrides. NeuTTS
   can normalize official sample-style `.pt` and compatible `.npy` codes but
   still cannot encode an arbitrary reference WAV. A real managed-Parakeet
   browser smoke test plus
   any LAN/HTTPS story remain open (managed browser audio: R24; NeuTTS: R17;
   LAN/HTTPS: R18).

## Roadmap to parity

Ordered roughly by leverage. Each step keeps the cross-cutting rules below.

1. **Bootstrap installer — done** (`install.ps1`, `update.ps1`): bare-machine
   WinGet/Go/toolchain provisioning, complete first-party Go binary build, data
   folder, LLM/voice choices, atomic non-secret state, choice-preserving source
   updates, launcher, and launch. This is the entry point until releases exist.
2. **Model-manager foundation (done).** Durable model inventory, provider model
   list, standalone GGUF import, configurable Ollama-path scan/import,
   checksum verification, cancellation, selection, and guarded removal.
3. **Packaged releases (Phase 16).** Windows portable zip / setup binary so the
   installer can *download a prebuilt binary* instead of building from source —
   no Go required for end users. Production signing remains an explicit Phase
   16 decision. Linux/macOS artifacts are best-effort.
4. **Managed llama.cpp runner provisioning (done for source installs).** The
   installer provisions missing Git/CMake/MSVC/CUDA build dependencies, and the
   installer and Model UI invoke one embedded helper pinned to `b9966` /
   `c749cb0`, verify the checkout and executable, build CPU or CUDA, install the
   complete runtime atomically, and activate a constrained app-data manifest.
   No user path setting remains. Phase 16 must publish checksummed prebuilt CPU
   and CUDA runtime bundles so release users do not need Git/CMake/Visual Studio;
   source build remains an advanced fallback.
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
8. **Voice setup (implemented adapters and source provisioning).** Provider
   selection, workers, push-to-talk, browser playback, and app-managed Parakeet
   and NeuTTS runtime installation have landed. Local WAV-to-reference-code
   generation has landed through a pinned native ONNX worker; prebuilt
   provisioning and broader microphone/provider compatibility checks still
   require release evidence.
   Providers stay optional and off the core runtime path (ADR 0007).
9. **Cross-platform + LAN.** Linux/macOS install scripts; decide the LAN/mobile
   HTTPS story explicitly before promising phone use (risk R18).

## Cross-cutting rules

These hold for every step above (from `docs/goals-and-guardrails.md` and
`AGENTS.md`):

- **Downloads are explicit user actions** with visible size, license, checksum,
  and disk-use; show compact inline progress, retry transient failures, retain
  resumable partials, verify before install, and move files atomically. Startup
  and status checks must never kick off a multi-GB download.
- **Build-tool installation is explicit too.** WinGet package agreements are
  accepted only after the script names the package, purpose, license, and large
  disk impact. `-Yes` is the unattended form of that consent.
- **The core stays pure-Go.** CUDA/torch and any native ML live in the external
  runner or a worker process, never in the MagicHandy binary.
- **Secrets never touch logs or the catalog** — the connection key and any API
  keys stay local and redacted.
- **Provider choice is visible state**, not a silent mid-session switch, and the
  app runs (chat aside) even with no model configured.

## Parity checklist

| StrokeGPT-ReVibed setup capability | MagicHandy status | Where |
| --- | --- | --- |
| One-command environment setup | **Implemented for source installs** — bootstraps dependencies and compiler | `install.ps1` |
| No Python/venv/torch to install | **Implemented for core, Parakeet, ElevenLabs, NeuTTS synthesis, and WAV reference encoding** | by design + R17 |
| Prebuilt one-click download | Planned | Phase 16 |
| LLM runner provisioning (CUDA/CPU) | **Implemented for source installs** | installer + Settings > Model |
| Model selection + local/Ollama import UI | **Implemented** | Settings > Model |
| Curated model download UI | Planned | catalog + Settings > Model |
| GPU/VRAM-aware recommendations | CUDA provisioning implemented; model/VRAM advice remains | installer + future catalog |
| Start/Stop convenience | `Start-MagicHandy.ps1` (opt-in) | install.ps1 |
| First-run setup wizard | Planned (script is the stand-in) | step 7 |
| Voice model setup | Partial - Parakeet and the NeuTTS runner/decoder/backbone/encoder are app-managed; Settings generates a reference from WAV with audio preview; prebuilt release provisioning remains | Phase 13 + Phase 16 prebuilt provisioning |
| LAN/mobile HTTPS helper | Undecided (scope in R18) | step 9 |

## Related docs

- `install.ps1` — the interactive installer.
- `update.ps1` — the choice-preserving source updater.
- `scripts/installer/InstallerSupport.psm1` — shared provisioning/state logic.
- `docs/source-installer.md` — package IDs, state schema, commands, and updater contract.
- `IMPLEMENTATION_PLAN.md` — Phase 16 (packaging), Phases 12-13 (voice).
- `docs/model-management.md` — model catalog and llama.cpp runner strategy.
- `docs/risk-register.md` — R13 (runner/model management), R18 (LAN/HTTPS).
- `docs/goals-and-guardrails.md` — the download/pure-Go/secret rules above.
