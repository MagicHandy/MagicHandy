# Windows Source Installer

`install.ps1` builds and configures MagicHandy from source. `update.ps1`
fast-forwards a clean checkout, preserves the install choices, rebuilds, and
relaunches it. Both scripts share
`scripts/installer/InstallerSupport.psm1`; they do not maintain parallel
provisioning logic.

The core install stays pure Go. Optional Python, PyTorch, and local TTS model
files are installed only through the separate scripts documented under
[Optional Local TTS](#optional-local-tts).

## Quick Start

From a PowerShell window in the directory where MagicHandy should be installed:

```powershell
git clone https://github.com/MagicHandy/MagicHandy.git
Set-Location .\MagicHandy
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\install.ps1
```

This is the same copy-paste block shown in the README.

## Supported Host

- 64-bit Windows 10 or Windows 11
- Windows PowerShell 5.1 or newer
- internet access for selected dependencies and models
- enough free space for the source toolchain and selected runtimes

The installer can start without Go, Git, CMake, a C++ compiler, CUDA, Ollama,
or Parakeet. Missing components are named and installed only after consent.
If WinGet is missing, the script offers Microsoft's supported App Installer
repair path.

## Normal Install Choices

The interactive flow asks for:

1. app/installer UI language;
2. built-in chat response language;
3. data directory and local HTTP port;
4. whether to configure a local LLM;
5. managed llama.cpp or Ollama;
6. CPU or CUDA for a managed llama.cpp build;
7. optional Ollama provisioning/model pull;
8. optional Parakeet runner and model; and
9. optional launcher creation.

Selecting managed llama.cpp explains why it is useful: MagicHandy owns a
pinned, known-compatible runtime and can load managed GGUF models directly.
Selecting Ollama avoids that source build and saves the compiler/runtime space
when the user already has a suitable Ollama installation.

## Provisioned Packages

| Selection | Package or tool | Purpose |
| --- | --- | --- |
| Always | `GoLang.Go` | Build the pure-Go app and worker adapters |
| Managed llama.cpp | Git | Fetch the pinned llama.cpp revision |
| Managed llama.cpp | `Kitware.CMake` | Generate the native build |
| Managed llama.cpp | Visual Studio Build Tools, Desktop C++ workload, Windows SDK | Compile the Windows runtime |
| Managed llama.cpp with CUDA | `Nvidia.CUDA` | Build NVIDIA acceleration |
| Ollama | `Ollama.Ollama` | Install the external local-model daemon when requested |
| Parakeet | pinned runner archive and GGUF model | Optional managed ASR |

Package agreements and large downloads are shown before installation.
`-Yes` is the unattended form of that consent. Every installed dependency is
verified before the build continues.

## Built Outputs

The source installer builds these binaries with `CGO_ENABLED=0`:

- `magichandy.exe`
- `voice-parakeet-worker.exe`
- `voice-openai-tts-worker.exe`
- `voice-elevenlabs-worker.exe`

Frontend production assets are built first and embedded in `magichandy.exe`.
Managed llama.cpp remains a separate native process and is never linked into
the core.

## Optional Local TTS

Local voice cloning is intentionally not coupled to the main installer or the
llama.cpp choice. After MagicHandy is built, run:

```powershell
.\scripts\install-tts-module.ps1
```

The module installer offers:

- Faster Qwen3-TTS for NVIDIA/CUDA systems; and
- Chatterbox Turbo for CPU or broader GPU compatibility.

It displays the pinned repository revision, license, model, hardware target,
loopback endpoint, destination, and multi-gigabyte download warning. After
consent it installs `uv` when needed, creates an isolated Python 3.11
environment under the MagicHandy data directory, downloads the model, and
uses `magichandy.exe -configure-tts-module` to persist the provider settings.
The Chatterbox launcher suppresses the upstream standalone browser so
MagicHandy remains the only UI.

Useful examples:

```powershell
.\scripts\install-tts-module.ps1 -PlanOnly -Module faster-qwen3-tts
.\scripts\install-tts-module.ps1 -Module faster-qwen3-tts -ReferenceWav C:\voices\sample.wav -ReferenceTranscript "Exact words in the sample." -AutoLaunch
.\scripts\install-tts-module.ps1 -Module chatterbox -Device cpu -AutoLaunch
.\scripts\update-tts-module.ps1
```

Faster Qwen3-TTS requires an NVIDIA GPU and CUDA. It cannot be selected with
`-Device cpu`. Chatterbox is the fallback for CPU operation.

`update-tts-module.ps1` reads `module-state.json`, preserves the existing
module, model, voice/reference, language, device, port, auto-launch, and
speak-replies choices, and asks at runtime whether to change them. It supports
`-PlanOnly` and does not update the main repository.

See `docs/voice-tts-modules.md` for endpoint and process-ownership details.

## Install Commands

Interactive:

```powershell
.\install.ps1
```

Inspect without changes:

```powershell
.\install.ps1 -PlanOnly
```

Unattended managed CUDA setup:

```powershell
.\install.ps1 -Yes -LlamaBackend cuda
```

Use Ollama and skip the managed llama.cpp build:

```powershell
.\install.ps1 -Yes -SkipLlamaBuild
```

Build without launching:

```powershell
.\install.ps1 -Yes -NoLaunch
```

Other important flags:

| Flag | Meaning |
| --- | --- |
| `-Port 49717` | Local loopback HTTP port |
| `-DataDir PATH` | SQLite, model, trace, and runtime data root |
| `-UILanguage LOCALE` | `en`, `es`, `pt-BR`, `zh-Hans`, or `ja` |
| `-ChatLanguage LOCALE` | Built-in response language |
| `-SkipParakeet` | Skip optional managed ASR assets |
| `-NoLauncher` | Do not create `Start-MagicHandy.ps1` |
| `-StatePath PATH` | Override install state for testing/managed use |

## Persisted Choices

By default, non-secret install state is stored at:

```text
%LOCALAPPDATA%\MagicHandy\install-state.json
```

Schema 2 records:

- repository and data paths;
- local port;
- UI and chat locales;
- whether LLM setup is enabled;
- managed llama.cpp choice and CPU/CUDA backend;
- Ollama choice and optional model;
- Parakeet choice; and
- launcher choice.

The Handy connection key, ElevenLabs key, OpenAI-compatible bearer key, and
other credentials never enter installer state, command output, or logs. Local
TTS module choices use a separate non-secret `module-state.json` inside that
module's install root.

## Update Behavior

Run:

```powershell
.\update.ps1
```

The updater:

- restores the saved UI language before displaying prompts;
- shows the previous choices and asks whether to modify them;
- refuses to update a dirty worktree;
- fast-forwards the current branch to its safe upstream target without
  discarding local commits;
- treats a merged feature branch with a deleted remote as eligible to advance
  from `origin/main` only after an ancestry check;
- explicitly takes controller ownership, sends Emergency Stop, and terminates
  only the app process tree belonging to this checkout;
- stages rebuilt binaries before replacing the active files;
- reapplies SQLite-backed language settings after a successful build; and
- opens the browser only after the new server owns the configured port and
  answers `/api/state`.

The updater does not install or change optional TTS modules. Run
`scripts/update-tts-module.ps1` for that module lifecycle.

## Language Recovery

Run:

```powershell
.\change-language.ps1
```

It displays native language names before locale-dependent text, updates both
installer state and app settings, and safely restarts a running app so an old
in-memory settings snapshot cannot overwrite the correction.

## Validation

`scripts/test-installer.ps1` covers:

- PowerShell syntax and localized catalog integrity;
- plan-only no-write behavior;
- package and build decision trees;
- install-state migration, validation, and atomic writes;
- managed llama.cpp versus Ollama plans;
- optional Parakeet plans;
- optional Faster Qwen3-TTS and Chatterbox plan/update behavior;
- complete Go binary output and staged replacement;
- process ownership, controller takeover, Stop, and relaunch behavior; and
- unsafe dirty-tree/update-state failures.

Release acceptance still requires a clean Windows VM run because a plan test
cannot prove WinGet repair, Visual Studio workload installation, CUDA driver
compatibility, model download reliability, or local TTS listening quality.
