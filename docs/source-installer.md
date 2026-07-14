# Source Installer And Updater

`install.ps1` is the Windows source-build bootstrap. `update.ps1` is its
choice-preserving updater. They share
`scripts/installer/InstallerSupport.psm1`; package detection, downloads, state,
and builds must not be reimplemented in either entry script.

This is not the Phase 16 prebuilt release path. It can begin with no development
tools installed, but a managed llama.cpp source build installs those tools on
the machine. It also builds and installs the coupled NeuTTS runtime. A future
packaged release avoids that source-toolchain footprint entirely.

## Supported Host

- 64-bit Windows 10/11 with Windows PowerShell 5.1 or newer
- internet access for missing packages and explicitly selected model assets
- permission to approve package-manager/UAC prompts

The embedded UI is already built, so Node.js is not an install dependency. The
core and first-party workers remain `CGO_ENABLED=0`; MSVC/CUDA/LLVM/Rust are used
only for external managed processes.

## Provisioned Packages

The script names each missing package, purpose, license, and large disk impact
before consent. `-Yes` is explicit unattended consent to those prompts.

| Selection | WinGet package | Why |
| --- | --- | --- |
| Always | `GoLang.Go` | Build the pure-Go core and worker adapters |
| Managed llama.cpp | `Git.Git` | Fetch the checksum-pinned source revision |
| Managed llama.cpp | `Kitware.CMake` | Configure/build `llama-server` |
| Managed llama.cpp | `Microsoft.VisualStudio.BuildTools` | Desktop C++ workload and Windows SDK |
| Managed llama.cpp + NeuTTS | `Rustlang.Rustup` | Build `stream_pcm` and its embedded CPU llama.cpp binding |
| Managed llama.cpp + NeuTTS | `LLVM.LLVM` | Provide `libclang` for generated llama.cpp Rust bindings |
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

These three voice executables are protocol adapters. The installer separately
provisions selected Parakeet assets and, whenever managed llama.cpp is selected,
an app-managed NeuTTS runtime. It still cannot create a reference voice:
licensed pre-encoded `.npy` codes and their exact transcript are required. A
successful source build therefore ends with **configuration required**, not a
claim that every provider is ready.

For manual/custom runtimes, Settings path fields provide **Browse...** on the
Windows host. This is a controller-gated, loopback-only native dialog; it does
not expose directory listings to remote browser clients. Typed paths remain
available for advanced or non-Windows setups.

Builds are written to per-process temporary executable paths and moved into
place only after Go succeeds. Before replacement, the installer finds only
`magichandy.exe` processes owned by the selected checkout, calls the local
Emergency Stop endpoint, and terminates that process tree (including optional
workers). A failed active Stop, unexpected port owner, or multiple checkout
instances aborts the rebuild and leaves every app in place. When no local motion
engine exists, an unreachable configured transport is surfaced as a warning
before teardown rather than misreported as delivered. Legacy Go `*.exe~`
backups are removed and ignored so an old in-use replacement cannot dirty the
checkout again.

Parakeet's external CPU runner and 644 MiB model are separate explicit assets.
The script displays their size/license, verifies their pinned SHA-256 values,
and installs them atomically under `<data-dir>/voice/parakeet`. It then prints
the deliberate activation sequence: Settings > Voice, select Parakeet and the
MagicHandy module, enable voice, save, then Start. Installing files never enables
or autostarts a microphone worker.

NeuTTS is coupled to the managed llama.cpp source-build choice. The installer:

1. installs LLVM/libclang and pinned Rust 1.94.0 for Windows MSVC through Rustup;
2. clones `neutts-rs` v0.1.1 and verifies commit
   `ae7ea9a2a8d93e63eacdc1f10522ad3f92cc725f`;
3. downloads the revision-pinned NeuCodec checkpoint and Air Q4 GGUF with fixed
   SHA-256 verification;
4. converts the checkpoint to `neucodec_decoder.safetensors` with the upstream
   pure-Rust converter, without Python or PyTorch;
5. builds `stream_pcm` with Cargo `--locked`, eSpeak, and its CPU
   `llama-cpp-4` binding; and
6. stages the runner/decoder and exact GGUF cache together, verifies their
   hashes, then swaps them atomically under `<data-dir>/voice/neutts/active`.

The active manifest records the built runner/decoder hashes, immutable model
revisions, source checkpoint hashes, and exact Rust compiler identity. Updates
reuse it without requiring Rustup only after the installer rehashes all active
artifacts. An interrupted directory swap restores the newest preserved backup
before retrying; rollback data is removed only after the replacement verifies.
When app-managed NeuTTS is selected, the app independently pins the Air Q4 cache
revision and rehashes the runner, decoder, and GGUF before it configures the
worker. Status polling never performs these large hashes.

The NeuTTS build does not reuse `llama-server.exe`; the upstream Rust runner
embeds its own llama.cpp binding. Coupling the choices shares the explicit
source-toolchain/download decision. It intentionally remains CPU-only even when
the chat runner uses CUDA, avoiding voice/LLM GPU contention. The runtime is
about 1.4 GiB installed; decoder conversion temporarily downloads another
approximately 1.1 GiB checkpoint and Cargo/build files can use several GB.
Skipping managed llama.cpp, including with `-SkipLlamaBuild`, skips Rustup,
`stream_pcm`, and all NeuTTS model work. Existing files are not deleted.

## Install Commands

Interactive setup:

```powershell
.\install.ps1
```

Inspect an unattended CUDA source-toolchain setup without changing the machine:

```powershell
.\install.ps1 -Yes -LlamaBackend cuda -PlanOnly -NoLaunch
```

Apply it:

```powershell
.\install.ps1 -Yes -LlamaBackend cuda
```

Use Ollama and avoid the managed llama.cpp/NeuTTS runtime toolchain:

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
- NeuTTS selection is derived from managed llama.cpp rather than stored as a
  separate choice
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
4. fetches explicitly, then fast-forwards `main` from `origin/main` or a live
   feature from its configured upstream;
5. if a feature upstream was deleted after merge, uses `origin/main` only when
   the local feature tip is already contained there; it never switches the
   branch or rewrites its upstream;
6. invokes the newly checked-out `install.ps1` with preserved or revised state;
7. during provisioning, sends Emergency Stop and stops this checkout's running
   app process tree before replacing binaries;
8. writes state only after provisioning succeeds; and
9. optionally launches the app, opening the browser only after `/api/state`
   confirms that the new process owns the configured port.

Windows PowerShell treats a non-2xx response as an exception, but the updater
still reads MagicHandy's JSON Stop payload. If motion was already inactive and
the payload confirms local stopped state, the updater still does not infer
physical delivery. Any unavailable, stale, or failed transport Stop requires the
operator to verify the device is physically stopped and type `STOPPED` in an
interactive update. Unattended updates, malformed responses, and unreachable
responses fail closed and leave the old app running.

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
CUDA/NeuTTS versus Ollama-only plans, app-managed NeuTTS manifest discovery, and
end-to-end plan-only install/update behavior.
Updater fixtures cover non-2xx Stop response parsing, strict response
validation, exact physical-stop confirmation, unattended refusal, `main`, a
live feature upstream, a single-branch
merged/deleted feature fallback, unmerged/deleted refusal, and dirty-tree
refusal. A real temporary app verifies Emergency Stop, checkout-scoped
process-tree teardown, and stale executable-backup cleanup. It intentionally
performs no package or model download.
