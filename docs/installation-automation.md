# Installation Automation — Parity Plan

## Goal

A non-technical user should be able to get MagicHandy running — app, LLM, model,
and device — with as little friction as StrokeGPT-ReVibed offered, ideally less.
This doc compares the two, records what already exists, and lays out the roadmap
to parity. It is the reference for the interactive installer (`install.ps1`) and
the eventual in-app setup wizard.

A structural advantage worth stating up front: MagicHandy's core is a single
pure-Go binary with **no Python, no venv, no pip, and no torch/CUDA in the core**.
The hardest, most failure-prone part of the old setup — building a Python ML
environment with matching CUDA/torch wheels — simply does not exist here. CUDA
matters only for the *external* llama.cpp runner, never for MagicHandy itself.

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
  NVIDIA GPU, gives guided LLM setup (Ollama pull or llama.cpp pointers) with
  **explicit, consented** downloads, optionally writes a `Start-MagicHandy.ps1`
  launcher, and opens the app.
- **External LLM by design:** the app talks to llama.cpp or Ollama; model paths
  and provider are set in Settings > Model. The core never downloads a model on
  startup.
- **Local data:** settings, memory, and prompt sets in a local SQLite DB; the
  Handy connection key is stored locally and never echoed back.

## Gaps vs the target

1. No packaged release yet — install still requires Go to build (Phase 16).
2. No automated llama.cpp runner provisioning (download the right CUDA/CPU build,
   verify, wire the path).
3. No in-app curated model catalog or guided model download with progress.
4. GPU handling is detection + advice only, not an end-to-end "pick the right
   runner and a model that fits your VRAM" flow.
5. No in-app first-run setup wizard (the installer script is the current
   stand-in).
6. Voice (ASR/TTS) setup and any LAN/HTTPS story are not built (Phases 12-13; R18).

## Roadmap to parity

Ordered roughly by leverage. Each step keeps the cross-cutting rules below.

1. **Bootstrap installer — done** (`install.ps1`): Go, build, data folder, LLM
   guidance, launch. The entry point until releases exist.
2. **Packaged releases (Phase 16).** Windows portable zip / signed build so the
   installer can *download a prebuilt binary* instead of building from source —
   no Go required for end users. Linux/macOS artifacts best-effort.
3. **Managed llama.cpp runner provisioning.** Download a pinned `llama-server`
   build, choosing a CUDA variant when an NVIDIA GPU is present and a CPU build
   otherwise; verify the checksum, place it, and set the path. Ties into
   `docs/model-management.md` and risk R13.
4. **Curated model catalog + guided download.** A small, opinionated list of
   recommended GGUF models with visible size, license, checksum, and rough VRAM
   fit; one-click download with progress; and "import a local GGUF" without a
   download. Startup and status checks never trigger a download (guardrail).
5. **GPU/VRAM aware recommendations.** Use `nvidia-smi` (and VRAM) to recommend a
   runner build and a model size that fits, with an honest CPU fallback and a
   note on driver/CUDA-toolkit expectations. Because CUDA lives only in the
   external runner, this stays far simpler than the old torch/CUDA install.
6. **In-app first-run setup wizard.** The eventual best UX and the real parity
   milestone for non-technical users: connect the Handy (enter key, test),
   choose a provider, pick/download a model, and confirm — all in the browser UI,
   superseding the script for most people.
7. **Voice setup (Phases 12-13).** ASR/TTS providers and their model downloads
   behind the worker boundary, optional and off the core install path (ADR 0007).
8. **Cross-platform + LAN.** Linux/macOS install scripts; decide the LAN/mobile
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
| LLM runner provisioning (CUDA/CPU) | Planned | R13, model-management |
| Model selection + download UI | Planned | catalog + Settings > Model |
| GPU/VRAM-aware recommendations | Detection + advice today | install.ps1 → step 5 |
| Start/Stop convenience | `Start-MagicHandy.ps1` (opt-in) | install.ps1 |
| First-run setup wizard | Planned (script is the stand-in) | step 6 |
| Voice model setup | Planned | Phases 12-13 |
| LAN/mobile HTTPS helper | Undecided (scope in R18) | step 8 |

## Related docs

- `install.ps1` — the interactive installer.
- `IMPLEMENTATION_PLAN.md` — Phase 16 (packaging), Phases 12-13 (voice).
- `docs/model-management.md` — model catalog and llama.cpp runner strategy.
- `docs/risk-register.md` — R13 (runner/model management), R18 (LAN/HTTPS).
- `docs/goals-and-guardrails.md` — the download/pure-Go/secret rules above.
