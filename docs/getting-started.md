# Getting Started

Everything you need to install, update, and run MagicHandy from source. The
[README](../README.md) carries the short version; this page carries the detail.

## Windows installer (`install.ps1`)

From the project folder, in PowerShell:

```powershell
.\install.ps1
```

The source installer can start on a clean 64-bit Windows machine. Everything it
installs is named, licensed, and consented to before it happens:

- It repairs Windows Package Manager (WinGet) through Microsoft's supported
  path when needed, then installs and verifies Go.
- Choosing the **managed llama.cpp build** additionally provisions Git, CMake,
  the Visual Studio Desktop C++ workload, and CUDA when selected. That choice
  also provisions LLVM/libclang and pinned Rust 1.94.0 through Rustup, builds
  MagicHandy's persistent NeuTTS runner with the same CPU/CUDA backend, and
  installs a checksum-verified Air Q4 backbone, a decoder converted from a
  checksum-verified NeuCodec checkpoint, and a pinned ONNX reference encoder.
- Choosing **Ollama** instead avoids the managed source builds and also skips
  NeuTTS; the installer can provision Ollama too.
- The installer builds the core and all three first-party Go voice adapters.
  The optional Parakeet runner and its 644 MiB model remain a separate,
  checksum-verified prompt, and voice remains disabled until you enable it.
- For NeuTTS, users supply a reference WAV and its exact transcript; Settings
  generates the `.npy` reference codes locally without Python. Manual
  pre-encoded paths remain available under Advanced.
- Enabled speech input and **Speak chat replies** load their configured workers
  automatically on later app starts. Installing assets alone does not enable
  either feature.
- It can write a `Start-MagicHandy.ps1` launcher, and when setup is done the
  app opens at <http://127.0.0.1:49717>.

### Useful flags

| Flag | Effect |
| --- | --- |
| `-SkipLlamaBuild` | Choose Ollama; skip managed llama.cpp and NeuTTS |
| `-Yes` | Unattended: accepts the displayed third-party package licenses and large-download choices |
| `-LlamaBackend cuda` | Select the CUDA build of managed llama.cpp |
| `-PlanOnly` | Show the planned work without doing any of it |

Example unattended CUDA source-toolchain setup:

```powershell
.\install.ps1 -Yes -LlamaBackend cuda
```

The installer stores only non-secret choices in
`%LOCALAPPDATA%\MagicHandy\install-state.json`. Provider credentials and the
Handy connection key never enter installer state.

Package IDs, the state schema, and the full command reference live in
[source-installer.md](source-installer.md).

## Updating (`update.ps1`)

```powershell
.\update.ps1
```

The updater shows the saved data directory, port, llama.cpp/NeuTTS/Ollama
selection, Parakeet choice, and launcher choice, then asks whether to modify
them. It rebuilds through the same provisioning implementation as the
installer, and it:

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

One-click packaging, guided model download, and GPU/CUDA-aware recommendations
are being brought up to the polish of the original StrokeGPT app — see the
plan in [installation-automation.md](installation-automation.md) and the
first-run wizard design in [gui-installer.md](gui-installer.md).

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
- The browser UI ships prebuilt, so you don't need Node just to run the app.
  To work on the UI itself (Vite + React + TypeScript, built at build time and
  embedded in the binary), see [`web/`](../web/) and
  [ADR 0009](decisions/0009-react-frontend.md).

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
