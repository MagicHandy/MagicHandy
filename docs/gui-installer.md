# GUI Installer — Evaluation And Design (2026-07-11)

The Windows install story needs three things a portable zip and a console
script cannot deliver alone: a real install binary (shortcuts, uninstall
entry, no Go toolchain), a **heavily interactive** setup experience (choose
whether to build llama.cpp, pick and download LLM/voice models with sizes and
licenses visible, enter cloud keys, port data from StrokeGPT-ReVibed), and
the existing consent/checksum law applied to all of it. This doc evaluates
how to get there and records the decision. It extends
`docs/installation-automation.md` (which anticipated "the eventual in-app
setup wizard") and feeds Phase 16.

## What already exists (and changes the answer)

The hard installer machinery is already **inside the app, behind the API,
with progress reporting**, as of PRs #55/#56:

- Pinned llama.cpp **source builds** with backend choice (auto/CPU/CUDA),
  queued/building/complete/failed/cancelled states, cancellation, and
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
  `installer/magichandy.iss`); nothing ships at runtime. The stub adds only
  a couple of MB around the app payload.
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
- Uninstall removes program files and shortcuts, **leaves the data
  directory** (settings, database, models — possibly tens of GB, and
  private) with the path shown; purging data stays a deliberate manual act.
- The portable zip remains the second official artifact for
  no-install/USB use; both come from the same release workflow.

## First-run onboarding wizard (`#/setup`)

Trigger: fresh data directory (no settings/database) or an explicit
`-setup` flag; re-runnable later from Settings. Every step is skippable and
non-blocking — the app must remain fully usable with everything declined
(voice optional, Ollama instead of managed builds, no migration).

1. **Welcome / consent** — what setup will and won't do: nothing downloads
   or builds without an explicit per-item click showing size and license;
   nothing here ever commands the device.
2. **Device** — connection key (write-only), dispatch owner, non-motion
   connection check. Existing settings surface, embedded.
3. **LLM runtime** — the choice the user asked for by name: managed
   llama.cpp **source build** (backend auto/CPU/CUDA, streaming status,
   cancel — existing endpoint), **skip because Ollama** (scan + import from
   the local Ollama store — existing endpoints), or external server URL.
4. **LLM model** — import GGUF / import from Ollama (existing), or the
   Phase 16 curated checksum-pinned downloads with hardware-fit
   recommendations once those land.
5. **Voice (optional)** — Parakeet ASR: download the pinned parakeet.cpp
   runner + model (sizes, licenses, SHA-256 — lifted from `install.ps1`
   into API endpoints so the wizard and the script share one checksummed
   path). NeuTTS: runner + reference codes with the same rules. ElevenLabs:
   write-only key entry. Speak-replies and push-to-talk explained.
6. **Port from StrokeGPT-ReVibed** — detect likely install locations, offer
   a browse fallback, then run the Phase 15 importer in **dry-run** first:
   per-category preview (settings, memories, prompt sets/personas, motion
   patterns, programs/funscripts) with counts and the compatibility report,
   per-category opt-in, then the real non-destructive import. Secrets land
   in the redacted store and are never echoed.
7. **Finish** — where things live (data dir, local URL, docs), what was
   skipped and where to do it later.

Invariants: all mutating steps sit behind the controller lease like every
other surface; downloads are server-side, checksum-pinned, size/license
visible, individually consented; the wizard adds no second copy of any
operation — every step is the existing settings/API surface arranged in
order.

Presentation: the default stays "binary opens the default browser at the
local URL" — now landing on `#/setup` on first run. A WebView2 app-window
shell (installer-app feel, no browser chrome) is deliberately deferred to
the Phase 16 decision docs; it is presentation only and must not change
where the logic lives.

## Gaps this plan closes (the actual work)

| Gap | Where it lands |
| --- | --- |
| Release plumbing: portable zip, version metadata, release workflow | Slice 16.0 (already in scope) |
| Inno Setup script, CI compile, uninstall story, over-install upgrade | Slice 16.1 |
| First-run detection, `#/setup` wizard route and steps, re-run from Settings | Slice 16.2 |
| Voice provisioning endpoints (checksummed downloads with progress, mirroring `install.ps1`) | Slice 16.2 |
| Curated LLM downloads + hardware-fit recommendations | Phase 16 scope (already listed) |
| StrokeGPT-ReVibed importer API (dry-run, report, non-destructive) | Phase 15 (wizard consumes it in slice 16.3) |
| Signing / auto-update / WebView2 shell decisions | Phase 16 decision docs (already listed) |

`install.ps1` stays for source-first developers and unattended installs, and
shrinks over time to a wrapper that fetches a release and defers to the same
in-app setup.
