# GUI Installer — Decision And Implemented Design

The Windows install story needs three things a portable zip and a console
script cannot deliver alone: a real install binary (shortcuts, uninstall
entry, no Go toolchain), a **heavily interactive** setup experience (choose
whether to install managed llama.cpp, pick and download LLM/voice models with sizes and
licenses visible, enter cloud keys, port data from StrokeGPT-ReVibed), and
the existing consent/checksum law applied to all of it. This doc evaluates
how to get there and records the decision. It extends
`docs/installation-automation.md` (which anticipated "the eventual in-app
setup wizard") and feeds Phase 16.
[ADR 0011](decisions/0011-windows-installer-shell.md) records the
architecture decision; this document is the detailed implementation design.

Status 2026-08-03: the thin Inno shell, portable payload, first-run detection,
re-runnable seven-step GUI, unified optional llama.cpp/Parakeet/TTS install-plan
endpoint, release lifecycle tests, and tag-gated publication are implemented.
The alpha.6 public setup was withdrawn after a severe Defender classification.
Microsoft completed the exact-file review as `Not malware` and removed the
detection. Alpha.8 restores the hardened unsigned x64 setup through ADR 0014's
reviewed-public policy and exact-artifact lifecycle test. Verified upstream
llama.cpp bundles and app-owned, policy-tolerant managed Python setup with native
CPython virtual-environment launchers remain available. Curated model downloads,
trusted signing, and broader optional-component acceptance remain open.

## What already exists (and changes the answer)

The hard installer machinery is already **inside the app, behind the API,
with progress reporting**, as of PRs #55/#56:

- Pinned, checksum-verified llama.cpp **release installs** with backend choice
  (auto/CPU/CUDA), queued/running/complete/failed/cancelled states, cancellation, and
  manifest validation (`POST /api/llm/runtime/build`), plus an opt-out path
  for existing Ollama users.
- A checksummed **model store** with GGUF import, Ollama store scanning and
  import, byte-level progress, and ID-based selection
  (`/api/llm/models`, `/api/llm/imports/*`, `/api/llm/ollama/*`).
- The **Model settings UI** for all of the above in the embedded React
  frontend, already conforming to the design system.
- Consent plumbing precedent: `install.ps1`'s voice provisioning downloads
  only after explicit consent with size and license shown and SHA-256
  verification (parakeet.cpp runner + model), and the Phase 15 scope defines
  the StrokeGPT-ReVibed importer (dry-run, compatibility report,
  non-destructive, secrets redacted).

Any external installer UI would have to re-implement or remote-control all
of this. That observation drives the recommendation.

## Options considered

**A. Extend `install.ps1` interactivity.** A console wizard is not a GUI; it
stays as the scripted/unattended path (`-SkipLlamaBuild` etc.), not the
answer to this requirement.

**B. Native installer framework with custom wizard pages (Inno Setup / NSIS
/ WiX-MSI).** These excel at OS integration — install dir, Start Menu,
Add/Remove Programs, upgrades — and are miserable hosts for heavy
interactivity. Streaming a CUDA build log, rendering a model catalog with
sizes/licenses/progress, or previewing a migration report means writing
substantial UI in Pascal script (Inno), NSIS script, or MSI dialog tables —
a second, worse UI stack outside the design system, duplicating logic the
app already has. Rejected as the *interactive* surface; Inno remains
relevant as the thin shell (below).

**C. Dedicated GUI installer app (Electron / Tauri / WebView2 native).**
Rich UI, but a second application with its own framework: Electron's
~80 MB+ runtime is absurd next to a <30 MB app budget; Tauri drags in a Rust
toolchain; a hand-rolled WebView2 host is real Win32 code to maintain. And
the moment it needs build progress or model downloads it either duplicates
the logic or launches the app and calls its API — at which point it *is*
option D with an extra process. Rejected.

**D. The app is the installer (recommended).** A first-run onboarding wizard
(`#/setup`) in the embedded React UI, served by the same Go binary,
orchestrating the endpoints that already exist — delivered by a **thin
Windows setup binary** that does only what the app cannot do for itself:
choose an install directory, place the exe, create Start Menu/desktop
shortcuts and the uninstall entry, then launch the app into setup.

Why D wins: one UI stack (design system, tests, accessibility already paid
for); the heavy operations already have APIs with progress and cancellation;
every wizard step doubles as a permanent settings surface (re-runnable from
Settings, so "installer" features never rot separately from the app); the
consent/checksum law is enforced in one place, server-side; and vitest/Go
suites already know how to test it.

## The thin outer shell: Inno Setup

For the install binary itself, **Inno Setup** over WiX/NSIS/hand-rolled:

- Build-time-only dependency (CI installs `iscc`, compiles
  `installer/magichandy.iss`); nothing ships at runtime. Target only a few MB
  of installer overhead around the app payload and measure it in Phase 16.
- Mature Add/Remove Programs integration, silent flags (`/VERYSILENT`) for
  scripted installs, over-install upgrades, and code-signing hooks (signing
  itself stays a Phase 16 decision doc).
- Explicitly **no custom wizard pages** beyond directory/shortcut choices —
  all real interactivity lives in the app. The finish page launches
  MagicHandy, which opens the browser at first-run setup.
- WiX/MSI is enterprise plumbing (GPO, transforms) no one asked for; NSIS
  offers nothing over Inno here; a pure-Go self-extracting stub would
  hand-roll uninstall/ARP semantics for purity points — recorded as the
  fallback if avoiding third-party build tools ever becomes a requirement.
- Uninstall always removes program files and shortcuts, then makes app-data
  disposition explicit. Interactive uninstall recommends deleting the private,
  potentially multi-gigabyte `%APPDATA%\MagicHandy` tree for a clean reinstall
  but can preserve it. Silent uninstall purges unless `/KEEPUSERDATA` is passed.
  External Ollama/media/source paths and custom data directories are never
  inferred or deleted.
- The portable zip remains the second official artifact for
  no-install/USB use; both come from the same release workflow.

## First-run onboarding wizard (`#/setup`)

Trigger: fresh data directory (no settings/database) or an explicit
`-setup` flag; re-runnable later from Settings. Every optional feature is
skippable before installation and non-blocking — the app must remain usable with everything declined
(voice optional, Ollama instead of managed builds, no migration).

The steps below are the *contract* (what each step may do and through which
API). The **experience design** — the full user decision tree with defaults,
screen anatomy, visual treatment, and branding slots — lives in
[setup-wizard-design.md](setup-wizard-design.md), with a wireframe in
[setup-wizard-sketch.svg](setup-wizard-sketch.svg).

1. **Welcome / consent** — what setup will and won't do: nothing downloads
   or builds before the user selects components with size/license disclosure
   and explicitly continues to installation;
   nothing here ever commands the device.
2. **Device** — connection key (write-only), dispatch owner, non-motion
   connection check. Existing settings surface, embedded.
3. **LLM runtime** — the Recommended fresh-install default is a pinned managed
   **verified release** (backend auto/CPU/CUDA), with download and installed
   size visible. CPU downloads about 18 MiB. CUDA downloads about 628 MiB,
   installs about 1.1 GiB, and requires a compatible NVIDIA driver. Neither
   installs a compiler or CUDA Toolkit. **Use existing Ollama** is never
   selected implicitly and avoids that managed-runtime footprint; users may
   also choose an external compatible server or skip.
4. **LLM model** — import a local GGUF into the checksummed managed store,
   scan an existing Ollama library and explicitly copy one compatible model,
   choose a model exposed by an Ollama daemon, enter an external server model
   ID, or skip.
   Curated checksum-pinned downloads and hardware-fit recommendations remain
   planned and are not represented as available actions.
5. **Voice (optional)** — Parakeet ASR: download the pinned parakeet.cpp
   runner + model (sizes, licenses, SHA-256 — lifted from `install.ps1`
    into API endpoints so the wizard and the script share one checksummed
    path). Local TTS: choose Faster Qwen3-TTS for NVIDIA/CUDA, Chatterbox for
    broader hardware, or an existing OpenAI-compatible endpoint; show pinned
    source/model licenses, disk impact, reference requirements, and process
    ownership before installation. Installing assets does not enable or start voice;
    enablement is explicit and a separate Start action confirms model readiness.
    App-managed modules and custom local paths are separate choices. Reference
    WAV/transcript selection, enabling voice, and starting workers stay in
    Settings > Voice.
6. **Install** — submit the selected local components once, then show the
   backend-owned sequential queue, per-component state, bounded terminal
   output, cancellation, and retry.
7. **Finish** — where things live (data dir, local URL), what was
   skipped and where to do it later.

The Phase 15 importer does not exist, so setup contains no disabled or
placeholder migration step. Add it only with a real dry-run and
non-destructive importer API.

Invariants: all mutating steps sit behind the controller lease like every
other surface; downloads are server-side, checksum-pinned, and size/license
visible before one reviewed install plan starts. The wizard adds no second copy
of any operation — every step is the existing settings/API surface arranged in
order.

Presentation: the default stays "binary opens the default browser at the
local URL" — now landing on `#/setup` on first run. A WebView2 app-window
shell (installer-app feel, no browser chrome) is deliberately deferred to
the Phase 16 decision docs; it is presentation only and must not change
where the logic lives.

## Gaps this plan closes (the actual work)

| Gap | Where it lands |
| --- | --- |
| Release plumbing: setup EXE, portable ZIP, version metadata, PR artifacts, tag publication | Implemented; `v0.1.0-alpha.8` restores reviewed unsigned setup publication after Microsoft's `Not malware` determination |
| Prebuilt CPU/CUDA llama.cpp runtime bundles, manifests, checksums, licenses | Implemented with official `b9966` CPU and CUDA 12.4 assets |
| Inno Setup script, destination/shortcut choices, explicit retain/purge uninstall | Implemented and covered by release lifecycle acceptance |
| First-run detection, `#/setup`, re-run from Settings | Implemented |
| Parakeet and managed TTS provisioning jobs | Implemented; full hardware/listening acceptance open |
| Curated LLM downloads + hardware-fit recommendations | Open |
| StrokeGPT-ReVibed importer API | Not implemented; therefore absent from setup |
| Signing / auto-update / WebView2 / LAN policy | ADR 0014 permits a reviewed unsigned-alpha setup exception; trusted Authenticode and silent update remain open |

`install.ps1` stays for source-first developers and unattended installs. Its
plain path builds the core and defers interactive choices to `#/setup`.
`update.ps1` rebuilds only the core, preserves GUI-owned optional assets, and
can reopen setup instead of replaying old source-installer choices.
