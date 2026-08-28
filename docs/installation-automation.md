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
| Reviewed Windows setup EXE | Normal Windows user | Install the versioned payload, shortcuts, and uninstaller, then open the app; backend first-run state selects `#/setup` only for a fresh store |
| Portable ZIP | No-install or USB use | Provide the payload without Windows integration |
| `bootstrap.ps1` + `install.ps1` | Source user | Repair WinGet/Git, build the pure-Go core, then open `#/setup` |
| `install.ps1 -Yes ...` | Managed automation | Provision explicitly selected optional components without prompts |
| `update.ps1` | Existing source checkout | Safely fast-forward and rebuild the core; optionally open `#/setup/reconfigure` |
| Settings > General > Run setup again | Existing app | Revisit device, model, and voice choices through `#/setup/reconfigure` |

The setup EXE and portable ZIP require no Go, Node, Python, CMake, Visual
Studio, or CUDA installation to run the MagicHandy core. The unsigned setup EXE
is published for explicitly approved versions under ADR 0014 after Microsoft's
`Not malware` determination for alpha.6, an exact-artifact Defender scan, and
the full setup lifecycle. The exception is version-bound and still lacks
publisher identity. Optional selections can add
their own dependencies after explicit consent.

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
Managed TTS installation selects the worker shipped with the current app and
removes obsolete custom worker overrides. The setup job completes only after
the same readiness contract used by Settings > Voice validates the managed
Python executable, pinned upstream server, MagicHandy adapter, and required
model or voice files.

## Dependency Boundaries

The core and first-party adapters are pure Go and build with `CGO_ENABLED=0`.
Optional dependencies stay outside the core process:

| Choice | Additional dependencies | Why it may be chosen |
| --- | --- | --- |
| Managed llama.cpp release | Official checksum-pinned Windows bundle; compatible NVIDIA driver for CUDA | App-owned pinned runner, startup, diagnostics, and lifecycle without a compiler toolchain |
| Existing Ollama | Existing Ollama service only | Avoid managed-runtime storage and use an existing model library |
| Parakeet | Pinned `parakeet.cpp` runner and roughly 646 MiB GGUF model | Local speech recognition |
| Faster Qwen3-TTS | `uv`, managed Python 3.11, CUDA PyTorch, pinned source/model | Faster NVIDIA voice cloning |
| Chatterbox | `uv`, managed Python 3.10, PyTorch, pinned source/model | CPU fallback and broader NVIDIA compatibility |

The managed llama.cpp path downloads official `b9966` Windows artifacts with
fixed sizes and SHA-256 digests. CPU is approximately 18 MiB compressed; CUDA
is approximately 628 MiB compressed and 1.1 GiB installed. Neither path needs
Git, CMake, Visual Studio, MSYS2, or the CUDA Toolkit. Choosing existing Ollama
or skipping chat setup avoids the managed runtime's disk use.

Managed Faster Qwen keeps Hugging Face, `uv`, and Numba caches below its
app-owned module root. It materializes ordinary model files for runtime use so
clean Windows installs do not depend on Hugging Face snapshot links. The
installer runtime probe and every managed launch use
the same Numba cache path, so Python never needs to write compiled cache probes
into packaged dependencies. Managed Python servers are assigned to one owned
process tree; stopping or restarting the worker terminates both a launcher and
the interpreter descendants it created.

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

Implemented:

- public portable Windows ZIP plus a thin reviewed Inno Setup EXE from one
  versioned payload;
- artifact manifest, exact source revision, GPL license, and SHA-256 sums;
- read-only pull-request packaging plus a separate SemVer-tag publication
  workflow; the exact public setup receives install, upgrade, retain, purge, and
  clean-reinstall acceptance;
- fresh-store detection at `#/setup` plus an explicit, re-runnable
  `#/setup/reconfigure` route that stale tabs cannot trigger after an update;
- GUI-managed verified llama.cpp installation, GGUF import, Ollama/external selection,
  Parakeet, Faster Qwen3-TTS, and Chatterbox provisioning;
- one cancellable backend setup queue with controller ownership;
- source bootstrap and updater delegation to the GUI; and
- packaged helper scripts so optional modules remain repairable after install.

Still open:

- broader clean-machine acceptance of every optional model and voice path;
- curated checksum-pinned GGUF downloads and hardware-fit recommendations;
- production code signing and publisher identity;
- any automatic update implementation;
- installer/account GUI and automatic CA/trust support for Phase 20's
  operator-configured LAN HTTPS backend; and
- a Phase 15 legacy importer, so no migration step appears in setup.

## Acceptance

- A standard user can extract and run the public portable core without a
  developer toolchain. The setup lifecycle remains continuously tested while
  public setup publication is signing-gated.
- The portable ZIP and CI setup candidate report the same version and derive
  from the same staged release manifest.
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
