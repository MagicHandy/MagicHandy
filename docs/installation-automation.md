# Installation Automation

## Goal

MagicHandy must be installable on a clean 64-bit Windows machine without
requiring the user to discover dependencies manually. The normal path has one
interactive owner: the embedded setup flow at `#/setup`. PowerShell and Inno
Setup provide bootstrap, operating-system integration, and unattended entry
points; they do not maintain competing product-choice screens.

This policy supersedes the earlier console-first parity plan. It follows
[ADR 0011](decisions/0011-windows-installer-shell.md) and is implemented in
Phase 16.

## Entry Points

| Entry point | Intended user | Responsibility |
| --- | --- | --- |
| Windows setup EXE | Normal Windows user | Install the versioned payload, shortcuts, and uninstaller, then open `#/setup` |
| Portable ZIP | No-install or USB use | Provide the same payload without Windows integration |
| `bootstrap.ps1` + `install.ps1` | Source user | Repair WinGet/Git, build the pure-Go core, then open `#/setup` |
| `install.ps1 -Yes ...` | Managed automation | Provision explicitly selected optional components without prompts |
| `update.ps1` | Existing source checkout | Safely fast-forward and rebuild the core; optionally open `#/setup` |
| Settings > General > Run setup again | Existing app | Revisit device, model, and voice choices at any time |

The setup EXE and portable ZIP require no Go, Node, Python, CMake, Visual
Studio, or CUDA installation to run the MagicHandy core. Optional selections
can add their own dependencies after explicit consent.

## GUI-Owned Decisions

Guided setup owns:

1. app and chat reply language;
2. device transport, write-only Cloud key, and non-motion connection check;
3. Recommended managed llama.cpp, explicit existing Ollama, external
   llama.cpp, or skip;
4. managed GGUF import, explicit copy from an existing Ollama library, Ollama
   daemon model selection, or external model ID;
5. optional Faster Qwen3-TTS or Chatterbox installation and execution device;
6. optional Parakeet installation; and
7. one backend-owned installation/progress/terminal page, followed by the data
   directory and local address summary.

Every optional feature is skippable before the plan starts. Installing a voice module does not enable voice,
start microphone capture, speak text, connect hardware, or command motion.
Reference WAV and transcript selection remains in Settings > Voice.

## Dependency Boundaries

The core and first-party adapters are pure Go and build with `CGO_ENABLED=0`.
Optional dependencies stay outside the core process:

| Choice | Additional dependencies | Why it may be chosen |
| --- | --- | --- |
| Managed llama.cpp source build | Git; MSYS2 UCRT64 GCC/CMake/Ninja or Visual Studio C++ for CPU; Visual Studio C++ and CUDA Toolkit for CUDA | App-owned pinned runner, startup, diagnostics, and lifecycle |
| Existing Ollama | Existing Ollama service only | Avoid duplicate compiler/runtime storage and use an existing model library |
| Parakeet | Pinned `parakeet.cpp` runner and roughly 646 MiB GGUF model | Local speech recognition |
| Faster Qwen3-TTS | `uv`, managed Python 3.11, CUDA PyTorch, pinned source/model | Faster NVIDIA voice cloning |
| Chatterbox | `uv`, managed Python 3.10, PyTorch, pinned source/model | CPU fallback and broader NVIDIA compatibility |

The managed llama.cpp path currently builds from source. Prebuilt CPU/CUDA
runtime bundles remain planned; until they land, choosing that path installs a
large compiler toolchain. The GUI says so before the action. Choosing existing
Ollama or skipping chat setup keeps that toolchain off the machine.

## Downloads And Consent

- No model, runtime, Python environment, CUDA toolkit, or optional worker is
  downloaded during app startup or a status check.
- Runtime and voice screens collect choices without build buttons. Purpose,
  disk impact, hardware requirements, and source/model licenses are visible
  before one Continue action submits the complete local installation plan.
- Pinned downloads use the same verification and resumable paths as the
  source scripts. Parakeet verifies runner and model hashes; managed TTS pins
  source revisions and keeps resumable model caches.
- The backend owns one sequential cancellable setup queue and streams bounded
  per-component status plus terminal output to the UI. Cancellation retains
  safe partial downloads for retry.
- `-Yes` is the unattended equivalent of consent and is honored only when the
  caller explicitly selects optional features.

## Source Install And Update Contract

A plain `install.ps1` creates a bootstrap state containing only repository,
data-directory, port, locale, and launcher information. Optional legacy fields
remain in schema 3 for compatibility with unattended installs, but the normal
path initializes them to off and does not act on them.

`update.ps1` always performs a core-only provision. It preserves SQLite-owned
settings and optional assets, refreshes small launcher shims in recognized TTS
module roots, and asks one question: whether to open guided setup after the
rebuild. It never rebuilds llama.cpp, reinstalls Parakeet, or recreates a Python
environment merely because old installer state says that component once
existed.

Both scripts refuse unsafe source updates, stop only a verified app process
owned by the checkout, stage replacement binaries, and wait for the new server
before opening the browser.

## Current Status

Implemented on the Phase 16 branch:

- portable Windows ZIP and thin Inno Setup EXE from one versioned payload;
- artifact manifest, exact source revision, GPL license, and SHA-256 sums;
- read-only pull-request packaging plus a separate SemVer-tag publication
  workflow with portable, install, upgrade, retain, purge, and clean-reinstall
  acceptance;
- fresh-store detection and a re-runnable `#/setup` route;
- GUI-managed llama.cpp source build, GGUF import, Ollama/external selection,
  Parakeet, Faster Qwen3-TTS, and Chatterbox provisioning;
- one cancellable backend setup queue with controller ownership;
- source bootstrap and updater delegation to the GUI; and
- packaged helper scripts so optional modules remain repairable after install.

Still open:

- broader clean-machine acceptance of every optional model and voice path;
- prebuilt managed llama.cpp CPU/CUDA bundles;
- curated checksum-pinned GGUF downloads and hardware-fit recommendations;
- production code signing and publisher identity;
- any automatic update implementation;
- LAN/mobile HTTPS support; and
- a Phase 15 legacy importer, so no migration step appears in setup.

## Acceptance

- A standard user can install, run, and uninstall the core from the setup EXE
  without a developer toolchain.
- The portable ZIP and setup EXE report the same version and contain the same
  release manifest.
- Silent setup and uninstall work; silent uninstall purges the default app-owned
  data root, `/KEEPUSERDATA` preserves it, and interactive uninstall asks.
- A fresh app opens setup, an existing settings document does not, and setup is
  re-runnable from Settings.
- Every optional job is explicit, cancellable, controller-gated, and leaves the
  app usable when skipped or failed.
- Source `install.ps1` works with no preinstalled dependencies; `update.ps1`
  preserves GUI choices and optional assets.
- Standard Go, frontend, PowerShell, race, lint, pure-Go, packaging, and visual
  checks are green.

## Related Docs

- [Windows release packaging](windows-release-packaging.md)
- [Windows source installer](source-installer.md)
- [GUI installer design](gui-installer.md)
- [Setup wizard design](setup-wizard-design.md)
- [ADR 0011](decisions/0011-windows-installer-shell.md)
- [Risk register](risk-register.md)
