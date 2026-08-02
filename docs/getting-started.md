# Getting Started

Everything you need to install, update, and run MagicHandy from source. The
[README](../README.md) carries the short version; this page carries the detail.

## Windows setup package

The current unsigned Windows x64 setup EXE and portable ZIP are published as
[v0.1.0-alpha.4](https://github.com/MagicHandy/MagicHandy/releases/tag/v0.1.0-alpha.4).
The setup EXE defaults to `C:\Program Files\MagicHandy`, exposes the destination
chooser and an optional desktop shortcut, and installs the prebuilt core,
Start Menu shortcut, and uninstaller without requiring Go, Node, Python, CMake,
or Visual Studio. It then opens the same guided setup described below. Verify
the download with the release's `SHA256SUMS` file. Build and acceptance commands
are in [windows-release-packaging.md](windows-release-packaging.md).

Versioned builds check the latest stable GitHub Release
and notify through the app. **Settings > General > Updates** provides an
explicit check and a manual-only preference. The app opens the release page;
it does not silently download or run setup. Installing a newer setup EXE over
the existing package preserves `%APPDATA%\MagicHandy`. Interactive uninstall
asks whether to remove that app-owned tree; choose **Yes** for a clean reinstall.
Full behavior and trust boundaries are in
[update-checks.md](update-checks.md).

## Windows source bootstrap (`install.ps1`)

On a machine without Git or other development dependencies, open PowerShell in
the desired parent folder and run:

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$bootstrap = Join-Path $env:TEMP 'MagicHandy-bootstrap.ps1'
Invoke-WebRequest -UseBasicParsing 'https://raw.githubusercontent.com/MagicHandy/MagicHandy/main/bootstrap.ps1' -OutFile $bootstrap
powershell.exe -NoProfile -ExecutionPolicy Bypass -File $bootstrap
```

From an existing project checkout:

```powershell
.\install.ps1
```

The source bootstrap can start on a clean 64-bit Windows machine. With no
feature flags, it installs or repairs only Git, WinGet, and Go as needed, builds
the core and first-party Go workers, writes a launcher, and opens `#/setup`.
App language, chat language, device, model runtime, model, Parakeet, and local
TTS choices are made in that GUI and remain available from Settings later.

This split keeps one interactive decision tree. The PowerShell scripts retain
explicit flags for unattended or managed deployments, but a normal install no
longer asks a long sequence of console questions.

The guided setup explains each optional path before it runs:

- The source bootstrap repairs Windows Package Manager (WinGet) through
  Microsoft's supported path when needed, then installs and verifies Go.
- Choosing the **managed llama.cpp runtime** downloads an official,
  checksum-pinned CPU or CUDA bundle. It does not install Git, CMake, Visual
  Studio, MSYS2, or the CUDA Toolkit.
- Choosing an **existing Ollama** install avoids the managed llama.cpp runtime
  and its disk use.
- Managed llama.cpp is the tightly controlled path: MagicHandy pins and tunes
  the runner and owns startup, model loading, structured replies, and
  diagnostics. Ollama is the simpler space-saving choice when an existing
  installation and model library should remain externally managed.
- CUDA normally makes local LLM and TTS inference much faster on a supported
  NVIDIA GPU. It also consumes disk and VRAM and requires a compatible driver;
  the managed llama.cpp CUDA bundle needs no separately installed Toolkit.
- The bootstrap builds the core and all first-party Go voice adapters.
  The optional Parakeet runner and its roughly 646 MiB model are a separate,
  checksum-verified GUI action, and voice remains disabled until you enable it.
- Optional Faster Qwen3-TTS and Chatterbox local speech modules are choices in
  guided setup. The selected path bootstraps WinGet, uv, managed
  Python, PyTorch, and its model as needed. Those files remain isolated and
  never become normal app or llama.cpp dependencies.
- Enabled speech input and **Speak chat replies** load their configured workers
  automatically on later app starts. Installing assets alone does not enable
  either feature.
- It can write a `Start-MagicHandy.ps1` launcher, and when setup is done the
  app opens at <http://127.0.0.1:49717>.

### Useful flags

| Flag | Effect |
| --- | --- |
| `-UILanguage ja` | Set the installer and app UI locale (`en`, `es`, `pt-BR`, `zh-Hans`, or `ja`) |
| `-ChatLanguage es` | Select the matching built-in chat reply prompt |
| `-SkipLlamaBuild` | Choose Ollama and skip managed llama.cpp |
| `-Yes` | Unattended: accepts the displayed third-party package licenses and large-download choices |
| `-LlamaBackend cuda` | Select the CUDA build of managed llama.cpp |
| `-TTSModule chatterbox` | Install managed local Chatterbox (`faster-qwen3-tts` is the NVIDIA cloning-quality path) |
| `-TTSDevice cpu` | Keep Chatterbox off the GPU (`cuda` is faster on supported NVIDIA hardware) |
| `-PlanOnly` | Show the planned work without doing any of it |

Example unattended CUDA source-toolchain setup:

```powershell
.\install.ps1 -Yes -LlamaBackend cuda
```

The installer stores only non-secret choices in
`%LOCALAPPDATA%\MagicHandy\install-state.json`. Provider credentials and the
Handy connection key never enter installer state.

The UI and chat locales are stored with those choices and applied to the real
SQLite-backed app settings after a successful build. If the wrong language was
selected, run:

```powershell
.\change-language.ps1
```

The recovery script shows native language names before any language-dependent
text. If this checkout is running, it sends Emergency Stop and safely restarts
the app so an old in-memory settings snapshot cannot overwrite the change.

Package IDs, the state schema, and the full command reference live in
[source-installer.md](source-installer.md).

## Optional local TTS

Guided setup offers local cloning directly. It requires no preinstalled
Python, uv, PyTorch, or compiler: Faster Qwen receives managed Python 3.11,
while Chatterbox receives Python 3.10 for its prebuilt Windows wheels. The
standalone entry point remains available:

```powershell
.\scripts\install-tts-module.ps1
```

The script offers Faster Qwen3-TTS for NVIDIA/CUDA systems and Chatterbox
Turbo as the CPU/broader-hardware fallback. It repairs WinGet if needed and
shows the pinned source, license, model, destination, and multi-gigabyte
download warning before
consent, creates an isolated `uv` environment, and writes the selected
provider and auto-launch choice into MagicHandy's SQLite settings. Use
`-PlanOnly` to inspect the operation without changing the machine.

Setup verifies real Git and `uv` executables instead of trusting aliases,
installs constrained dependency versions, and tests the final Python/native
runtime and selected CUDA backend before downloading a model. Native SoX and
FFmpeg are not required for the managed 12 Hz Qwen and WAV-only Chatterbox
paths.

For Faster Qwen, the command-line step installs only the runtime and model.
Add the reference WAV and its exact transcript later in Settings > Voice; an
empty reference during installation is expected and does not fail setup.

To update an installed module while preserving its choices:

```powershell
.\scripts\update-tts-module.ps1
```

Full provider behavior, reference-audio requirements, and ownership rules are
in [voice-tts-modules.md](voice-tts-modules.md).

## Updating (`update.ps1`)

```powershell
.\update.ps1
```

The updater restores the saved UI language, shows the source-bootstrap state,
and asks only whether guided setup should open after the rebuild. It updates
the core and small app-owned TTS launcher shims without rebuilding or deleting
managed llama.cpp, Parakeet, Python environments, models, or other GUI-owned
assets. It also:

- refuses to update over local source changes and only fast-forwards — `main`
  follows `origin/main`, a live feature branch follows its configured
  upstream, and a merged feature whose remote branch was deleted can safely
  advance from `origin/main` without switching branches or discarding work;
- sends Emergency Stop and terminates only the app process tree owned by this
  checkout before replacing the executable, staging binaries before
  replacement;
- opens the browser only after the rebuilt server owns the configured port and
  answers `/api/state`.

## Models

Settings > Model lists models reported by external llama.cpp/Ollama servers
and MagicHandy's managed GGUF copies. Managed llama.cpp resolves its runner
and selected model from app-owned inventories, never user-entered
executable/model paths. You can import a standalone GGUF, or scan an existing
Ollama models directory and explicitly copy a compatible model — imports show
progress, verify SHA-256, and never modify the Ollama library. Models are
never bundled or downloaded at startup.

The Windows setup binary, portable ZIP, and first-run setup flow are documented
in [windows-release-packaging.md](windows-release-packaging.md) and
[gui-installer.md](gui-installer.md). Curated downloads and model-fit guidance
remain planned.

## Build and run by hand

Requires [Go](https://go.dev/dl/) 1.25 or newer.

```powershell
go run ./cmd/magichandy
```

- The app serves its UI at <http://127.0.0.1:49717>; pass
  `-addr 127.0.0.1:PORT` to change the port.
- Your data lives under your OS config directory
  (`MagicHandy/magichandy.db`); pass `-data-dir .\.local-data` to keep it
  somewhere else.
- If SQLite detects physical corruption at startup, MagicHandy preserves the
  exact database and sidecars under the data directory's `recovery` folder,
  starts a fresh schema, and reports that backup path in app status and logs.
  Keep that recovery folder when collecting support evidence.
- The browser UI ships prebuilt, so you don't need Node just to run the app.
  To work on the UI itself (Vite + React + TypeScript, built at build time and
  embedded in the binary), see [`web/`](../web/) and
  [ADR 0009](decisions/0009-react-frontend.md).

## End-of-turn review handoff

Every completed contributor or agent turn ends with a reviewable app, including
docs-only turns:

- Ensure a process representing the current worktree is running. A healthy
  existing process may be reused when app source and embedded assets have not
  changed; otherwise build or run the current tree.
- Leave a browser open at the route and UI state most relevant to the completed
  work. Preserve the user's current route when that is the intended review
  state; do not hand off only a terminal, a blank tab, or an unrelated default
  page.
- Verify `/healthz` before handoff, then include the exact review URL and visible
  state in the final response. Leave both the app process and browser tab
  running after the response.
- Do not stop or replace a user-launched app session without explicit
  permission. If it does not represent the current worktree, start a parallel
  instance with `-addr 127.0.0.1:PORT -data-dir .\.scratch\review-data` on an
  unused port and open that instance instead.
- A review handoff never authorizes device discovery, connection, or motion.
  Keep hardware idle unless the user separately requested a live-device test.

## Validate a change

Before pushing:

```powershell
go fmt ./...
go vet ./...
go test ./...
go test -race ./...          # needs a C compiler; CI also runs it on Linux
$env:CGO_ENABLED = "0"; go build ./cmd/magichandy
npm --prefix web ci
npm --prefix web run typecheck
npm --prefix web run test
npm --prefix web run build
```

The shared standards for contributors — device safety, the pure-Go core,
import boundaries, secret/data hygiene — are in [AGENTS.md](../AGENTS.md).
