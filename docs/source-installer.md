# Windows Source Installer

`install.ps1` bootstraps and builds MagicHandy from source. Its normal path
then opens the app-owned guided setup instead of maintaining a second console
decision tree. `update.ps1` fast-forwards a clean checkout, rebuilds the core,
and optionally reopens guided setup. Both scripts share
`scripts/installer/InstallerSupport.psm1`; they do not maintain parallel
provisioning logic.

The core install stays pure Go. Guided setup can provision an optional local
TTS module, but its managed Python, PyTorch, and model files remain isolated
under the data directory. The same module lifecycle is also available through
the scripts documented under
[Optional Local TTS](#optional-local-tts).

## Quick Start

From a PowerShell window in the directory where MagicHandy should be installed:

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$bootstrap = Join-Path $env:TEMP 'MagicHandy-bootstrap.ps1'
Invoke-WebRequest -UseBasicParsing 'https://raw.githubusercontent.com/MagicHandy/MagicHandy/main/bootstrap.ps1' -OutFile $bootstrap
powershell.exe -NoProfile -ExecutionPolicy Bypass -File $bootstrap
```

This is the same copy-paste block shown in the README. `bootstrap.ps1` installs
Git after consent and creates the checkout before handing control to
`install.ps1`; users who already cloned the repository can run `.\install.ps1`
directly.

## Supported Host

- 64-bit Windows 10 or Windows 11
- Windows PowerShell 5.1 or newer
- internet access for selected dependencies and models
- enough free space for the app build and selected runtimes

The installer can start without Go, Git, CMake, a C++ compiler, CUDA, Ollama,
Parakeet, uv, Python, PyTorch, or a speech model. Missing components are named
and installed only after consent. If WinGet is missing, the script offers
Microsoft's supported App Installer repair path. A working NVIDIA driver is a
hardware prerequisite for CUDA paths; CPU Chatterbox remains available when no
NVIDIA GPU is present.

## Normal Install Choices

A plain `.\install.ps1` uses the profile data directory, port 49717, English
as the bootstrap locale, and creates the generated launcher. It does not ask
for device, model, runtime, CUDA, speech, or model-download choices. After the
core build is ready it opens `#/setup`, where the user can choose:

1. app and built-in chat response languages;
2. device transport and a write-only Handy key when Cloud REST is selected;
3. managed llama.cpp, an existing Ollama service, an external compatible
   server, or no chat setup;
4. a managed GGUF import, an explicit copy from an existing Ollama library, an
   Ollama daemon model, or an external model identifier;
5. optional Faster Qwen3-TTS or Chatterbox provisioning; and
6. optional Parakeet speech recognition;
7. one installation page with per-component progress and terminal output.

Managed llama.cpp is the Recommended fresh-install default; Ollama is never
selected implicitly. The GUI explains why managed llama.cpp is useful: MagicHandy owns a
pinned, tuned runtime and controls startup, GGUF loading, structured-response
behavior, and diagnostics. Selecting Ollama avoids that managed runtime and
saves its disk space when the user already has a suitable installation, at
the cost of less runner lifecycle control.

CUDA normally produces local LLM and TTS output much faster than CPU on a
supported NVIDIA GPU. Guided setup also states the cost: compatible drivers,
GPU memory, and about 1.1 GiB installed for managed llama.cpp. Its official
CUDA 12.4 bundle needs no CUDA Toolkit or compiler. TTS PyTorch wheels carry
their selected CUDA runtime and do not add a compiler dependency.

For automation, passing `-Yes` with explicit feature flags retains the
non-interactive provisioning path. Flags are not required for normal use.

## Provisioned Packages

| Selection | Package or tool | Purpose |
| --- | --- | --- |
| Always | `GoLang.Go` | Build the pure-Go app and worker adapters |
| Managed llama.cpp | Git | Fetch the pinned llama.cpp revision |
| Managed llama.cpp (CPU) | Existing or provisioned MSYS2 UCRT64 GCC, CMake, and Ninja; Visual Studio fallback | Compile the Windows runtime |
| Managed llama.cpp (CUDA) | `Kitware.CMake`, Visual Studio Build Tools, Desktop C++ workload, Windows SDK | Compile the accelerated Windows runtime |
| Managed llama.cpp with CUDA | `Nvidia.CUDA` | Build NVIDIA acceleration |
| Unattended `-SkipLlamaBuild` path | `Ollama.Ollama` | Install the external local-model daemon when explicitly requested by flags |
| Parakeet | pinned runner archive and GGUF model | Optional managed ASR |
| Managed local TTS | `astral-sh.uv`, managed Python 3.10 or 3.11, pinned Python packages and model | Optional isolated local speech |

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

Local voice cloning is independent of the llama.cpp choice and is offered in
guided setup. The provisioning scripts offer:

- Faster Qwen3-TTS for NVIDIA/CUDA systems; and
- Chatterbox Turbo for CPU or broader NVIDIA compatibility.

Selecting either path bootstraps WinGet when necessary, installs `uv`, installs
the required managed Python runtime, creates an isolated environment, and
downloads PyTorch and the model. No preinstalled Python or compiler is
required. Faster Qwen uses Python 3.11; pinned Chatterbox uses Python 3.10 so
Windows receives prebuilt Torch, torchvision, and ONNX wheels rather than
attempting native builds.

The same flow remains directly callable after MagicHandy is built:

```powershell
.\scripts\install-tts-module.ps1
```

It displays the pinned repository revision, license, model, hardware target,
loopback endpoint, destination, and multi-gigabyte download warning. After
consent it creates the module-compatible Python environment under the
MagicHandy data directory, downloads the model, and
uses `magichandy.exe -configure-tts-module` to persist the provider settings.
The Chatterbox launcher suppresses the upstream standalone browser so
MagicHandy remains the only UI.

On Windows, model files are finalized serially so Hugging Face's ordinary-file
fallback works without Administrator access, Developer Mode, or symlink
privileges. A failed model transfer is retried against the same resumable cache
and completed files are retained; rerunning the installer continues that cache
instead of starting the multi-gigabyte download over.

The module install is restartable before `module-state.json` exists. A retry
reuses the managed source checkout, Python environment, installed packages,
and model cache. Package metadata created by the install itself is registered
in that checkout's private `.git/info/exclude`; tracked source edits and any
other untracked files still stop the update instead of being overwritten. The
initial no-checkout clone is staged beside the final source directory and moved
into place only after Git succeeds. A retry recognizes and completes the empty
worktree left by older installers without treating Git's expected deleted-file
status as a user edit. Populated dirty worktrees remain protected. The
main installer runs module scripts in an isolated Windows PowerShell process.
Their support-module initialization therefore cannot invalidate the active
parent provisioner before launcher creation and saved-state commit.

Faster Qwen reference setup is deliberately not part of the command-line
installer. Installation completes with the runtime and model present, then
Settings > Voice accepts the reference WAV and its exact transcript. Until
both are saved, MagicHandy reports the module as installed but not yet ready
and does not launch its worker.

Useful examples:

```powershell
.\scripts\install-tts-module.ps1 -PlanOnly -Module faster-qwen3-tts
.\scripts\install-tts-module.ps1 -Module faster-qwen3-tts -AutoLaunch
.\scripts\install-tts-module.ps1 -Module chatterbox -Device cpu -AutoLaunch
.\scripts\update-tts-module.ps1
```

Faster Qwen3-TTS requires an NVIDIA GPU and CUDA. It cannot be selected with
`-Device cpu`. Chatterbox is the fallback for CPU operation.

`update-tts-module.ps1` reads `module-state.json`, preserves the existing
module, model, voice, language, device, port, auto-launch, and speak-replies
choices, and asks at runtime whether to change them. Reference settings remain
owned by the app database and are neither prompted for nor overwritten during
a module update. The script supports `-PlanOnly` and does not update the main
repository.

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
| `-TTSModule none|faster-qwen3-tts|chatterbox` | Select optional managed local TTS for unattended setup |
| `-TTSDevice auto|cpu|cuda` | Select the TTS execution device |
| `-NoTTSAutoLaunch` | Keep the selected TTS server externally managed |
| `-NoLauncher` | Do not create `Start-MagicHandy.ps1` |
| `-StatePath PATH` | Override install state for testing/managed use |

## Persisted Choices

By default, non-secret install state is stored at:

```text
%LOCALAPPDATA%\MagicHandy\install-state.json
```

Schema 3 records source-bootstrap and unattended-install history:

- repository and data paths;
- local port;
- UI and chat locales;
- whether LLM setup is enabled;
- managed llama.cpp choice and CPU/CUDA backend;
- Ollama choice and optional model;
- Parakeet choice;
- managed TTS module, CPU/CUDA target, and auto-launch choice; and
- launcher choice.

The Handy connection key, ElevenLabs key, OpenAI-compatible bearer key, and
other credentials never enter installer state, command output, or logs. The
module's model, packaged voice, language, port, and speak-replies choices
remain in a separate non-secret `module-state.json` inside its install root.
Faster Qwen reference paths and transcripts live only in the app settings
database after the user saves them in Settings > Voice; they are not duplicated
into installer state.

## Update Behavior

Run:

```powershell
.\update.ps1
```

The updater:

- restores the saved UI language before displaying prompts;
- shows the previous source-bootstrap choices and asks whether to open guided
  setup after the rebuild;
- refuses to update a dirty worktree;
- fast-forwards the current branch to its safe upstream target without
  discarding local commits;
- treats a merged feature branch with a deleted remote as eligible to advance
  from `origin/main` only after an ancestry check;
- explicitly takes controller ownership, sends Emergency Stop, and terminates
  only the app process tree belonging to this checkout;
- stages rebuilt binaries before replacing the active files;
- preserves managed runtimes and models without replaying stale installer
  choices;
- refreshes only the small app-owned launcher shim in every recognized managed
  TTS installation;
- preserves SQLite-backed GUI settings; and
- opens the browser only after the new server owns the configured port and
  answers `/api/state`.

Use Settings or `#/setup` to install or retarget optional features. An ordinary
app update does not reinstall them. Run
`scripts/update-tts-module.ps1` when intentionally updating the module's pinned
source, dependencies, model, reference, or other module-level choices.
Declining installer-managed TTS does not remove module assets or disable a
cloud/custom provider selected in Settings.

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
- GUI-default and unattended package/build plans;
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
