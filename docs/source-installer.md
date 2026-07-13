# Source Installer And Updater

`install.ps1` is the Windows source-build bootstrap. `update.ps1` is its
choice-preserving updater. They share
`scripts/installer/InstallerSupport.psm1`; package detection, downloads, state,
and builds must not be reimplemented in either entry script.

This is not the Phase 16 prebuilt release path. It can begin with no development
tools installed, but a managed llama.cpp source build installs those tools on
the machine. A future packaged release avoids that footprint entirely.

## Supported Host

- 64-bit Windows 10/11 with Windows PowerShell 5.1 or newer
- internet access for missing packages and explicitly selected model assets
- permission to approve package-manager/UAC prompts

The embedded UI is already built, so Node.js is not an install dependency. The
core and first-party workers remain `CGO_ENABLED=0`; MSVC/CUDA are used only for
the external managed llama.cpp process.

## Provisioned Packages

The script names each missing package, purpose, license, and large disk impact
before consent. `-Yes` is explicit unattended consent to those prompts.

| Selection | WinGet package | Why |
| --- | --- | --- |
| Always | `GoLang.Go` | Build the pure-Go core and worker adapters |
| Managed llama.cpp | `Git.Git` | Fetch the checksum-pinned source revision |
| Managed llama.cpp | `Kitware.CMake` | Configure/build `llama-server` |
| Managed llama.cpp | `Microsoft.VisualStudio.BuildTools` | Desktop C++ workload and Windows SDK |
| CUDA backend | `Nvidia.CUDA` | Provide `nvcc` and CUDA build/runtime files |
| Ollama selected | `Ollama.Ollama` | External local-LLM daemon/runtime |

If WinGet is absent, the installer offers Microsoft's supported
`Microsoft.WinGet.Client` repair flow. Every installed tool is resolved and
verified in the current process; the script asks for a restart/retry only when
Windows reports success but the executable or compiler workload is still not
available. For CUDA builds, the managed-runtime helper also derives the toolkit
root from `nvcc.exe` and passes it to both CMake and NVIDIA's Visual Studio build
targets, including when WinGet installed CUDA earlier in the same process.

## Built Outputs

Every successful run builds these files beside the source checkout:

- `magichandy.exe`
- `voice-parakeet-worker.exe`
- `voice-neutts-worker.exe`
- `voice-elevenlabs-worker.exe`

The executables and generated `Start-MagicHandy.ps1` are ignored build/runtime
artifacts. The optional portable `data/` directory is ignored too.

Parakeet's external CPU runner and 644 MiB model are separate explicit assets.
The script displays their size/license, verifies their pinned SHA-256 values,
and installs them atomically under `<data-dir>/voice/parakeet`.

## Install Commands

Interactive setup:

```powershell
.\install.ps1
```

Inspect an unattended full CUDA setup without changing the machine:

```powershell
.\install.ps1 -Yes -LlamaBackend cuda -PlanOnly -NoLaunch
```

Apply it:

```powershell
.\install.ps1 -Yes -LlamaBackend cuda
```

Use Ollama and avoid the managed llama.cpp runtime/toolchain:

```powershell
.\install.ps1 -Yes -SkipLlamaBuild
```

## Persisted Choices

After all selected provisioning succeeds, the installer atomically writes
`%LOCALAPPDATA%\MagicHandy\install-state.json` (or `-StatePath` for managed/test
use). Schema v1 contains only:

- install/update timestamps and repository path
- data directory and local port
- whether local LLM setup is selected
- managed llama.cpp selection and concrete CPU/CUDA backend
- Ollama selection and optional public model name
- Parakeet asset selection
- launcher selection

No API key, Handy connection key, prompt/chat content, model bytes, or voice
credential belongs in this file. The main SQLite settings database remains the
authority for application settings and credentials.

## Update Behavior

Run:

```powershell
.\update.ps1
```

The updater:

1. reads and displays the saved choices;
2. asks whether to modify them (default: no);
3. refuses to touch a dirty Git worktree;
4. runs `git pull --ff-only` on the configured current branch;
5. invokes the newly checked-out `install.ps1` with preserved or revised state;
6. writes state only after provisioning succeeds; and
7. optionally launches the app.

Use `-Yes -NoLaunch` for an unattended update with unchanged choices,
`-Reconfigure` to walk every choice, `-NoPull` to rebuild the current checkout,
or `-PlanOnly` to inspect the saved dependency graph without changing anything.
During reconfiguration, blank input keeps the displayed value; enter `-` to
clear the optional saved Ollama model choice.

Changing a choice never silently deletes an existing runtime, model library, or
voice asset. It only changes what subsequent runs ensure is present.

## Validation

`scripts/test-installer.ps1` runs under Windows PowerShell 5.1 in CI. It checks
all script syntax, atomic state round trips and secret-field exclusion, managed
CUDA versus Ollama-only plans, and end-to-end plan-only install/update behavior.
It intentionally performs no package or model download.
