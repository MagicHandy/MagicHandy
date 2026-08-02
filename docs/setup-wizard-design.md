# Setup Wizard Design

[gui-installer.md](gui-installer.md) records the architecture: Inno Setup is a
thin Windows shell and the embedded app owns interactive setup at `#/setup`.
This document describes the implemented seven-step experience and the remaining
work. The reference sketch is
[setup-wizard-sketch.svg](setup-wizard-sketch.svg); it is illustrative rather
than a second UI specification.

## Design Goals

1. **One decision owner.** Normal `install.ps1`, the setup EXE, and Settings all
   lead to the same embedded flow.
2. **Safe to skip.** Every optional step can be skipped and setup never
   commands motion, starts capture, or enables voice implicitly.
3. **Honest cost.** Runtime and voice choices show why they are useful, their
   license, hardware constraint, and approximate disk impact before install.
4. **Recoverable.** Setup is re-runnable from Settings and interrupted jobs can
   be cancelled or retried without reinstalling the core.
5. **Backend authoritative.** Settings, job state, model inventory, hardware
   detection, and connection results come from the backend.

## Entry And Completion

A fresh data store has `ui.setup_completed=false`, so any normal route redirects
to `#/setup`. Settings documents created before this field existed are treated
as already configured and are not forced through onboarding after an update.
The `-setup` flag opens the route explicitly, and Settings > General exposes
**Run setup again**.

Completion is an explicit backend write. Leaving halfway preserves each saved
step and keeps setup required. The normal app shell remains mounted throughout,
including the always-reachable Emergency Stop.

## Decision Tree

```text
1. Welcome
   - app language
   - chat reply language

2. Device
   - Handy Cloud REST + write-only connection key + non-motion check
   - Browser Bluetooth
   - Intiface Central
   - skip

3. Model runtime
   - managed verified llama.cpp runtime: auto / CPU / CUDA (default, Recommended)
   - existing Ollama service
   - external compatible llama.cpp server
   - skip chat setup

4. Model
   - select or import a GGUF for managed llama.cpp
   - scan an existing Ollama library and explicitly copy one compatible model
     into the managed store
   - select a model exposed by Ollama
   - enter the external server's model identifier
   - skip

5. Voice (both optional)
   - output: none / Faster Qwen3-TTS / Chatterbox / external compatible server
   - input: install Parakeet or skip

6. Install
   - review the selected local components as one plan
   - install sequentially with per-component state, progress, terminal output,
     Cancel, and Retry

7. Finish
   - data directory
   - selected runtime and voice path
   - local address and pre-motion reminder
```

The Phase 15 StrokeGPT-ReVibed importer is not implemented, so no migration
step or disabled placeholder is shown. Curated GGUF model downloads remain
absent until a real model catalog, hashes, licenses, and download handlers exist.

## Runtime Choice

The current managed option installs checksum-pinned official Windows bundles.
It is the fresh-install default and keeps the **Recommended** badge because it
gives MagicHandy a pinned, app-owned runner whose startup, model loading,
diagnostics, and shutdown are under application control. CPU downloads about
18 MiB. CUDA downloads about 628 MiB, installs about 1.1 GiB, and requires a
compatible NVIDIA driver and GPU. Neither option installs a compiler or CUDA
Toolkit. The screen states those costs before the user continues.

Ollama is never preselected or marked Recommended. It is an explicit option for
an existing installation and can save the managed runtime footprint by
using the user's daemon and model library. The model step also supports a
different workflow: read-only scanning of an Ollama library followed by an
explicit copy of one compatible GGUF into MagicHandy's checksummed managed
store. External llama.cpp is similarly user-owned. The runtime choice is saved
when leaving the runtime step, so skipping model selection does not silently
discard it.

## Voice Choice

Faster Qwen3-TTS is offered only when an NVIDIA GPU is detected and uses CUDA.
Chatterbox offers CPU and CUDA. Each module shows its code/model licenses,
approximate disk impact, and reference requirement. The server installation
can be configured for app auto-launch, but installing assets keeps voice and
Speak replies disabled.

Parakeet is a selectable component with runner/model license and download size
shown. Reference WAV and exact transcript selection, provider tuning,
enabling voice, and starting workers remain in Settings > Voice, where they can
be tested with the rest of the voice controls.

## Installation Jobs

Runtime, model, and voice pages collect choices; they do not expose independent
build buttons. Continuing from Voice submits one reviewed plan to the backend.
The backend runs managed llama.cpp, local TTS, and Parakeet sequentially through
one queue. A queued or running plan blocks another install and exposes bounded,
always-visible terminal output plus per-component state and progress. One Cancel
action owns the helper process tree and terminates it on cancellation or server
shutdown. Failed/cancelled plans remain visible after refresh and can be
retried; safe partial downloads remain available to the underlying installers.

All job-start and cancel endpoints require controller ownership. The GET status
endpoint is read-only and redacts credentials. Helper script arguments are
closed enums and app-owned paths; arbitrary shell commands are not accepted.

## Layout

Desktop uses a compact progress rail and one bordered setup work area inside
the existing shell. The main pane has a small step label, task-sized heading,
unframed explanatory copy, and flat radio choice rows. It does not use a
marketing hero, gradients, nested cards, glow, or decorative animation.

Below 780 px the progress rail becomes a seven-position top stepper. Below 560 px
two-column fields collapse and action buttons wrap. The job indicator respects
`prefers-reduced-motion`; all text and paths wrap rather than forcing horizontal
overflow.

The `M` tile and wordmark are current placeholders. A final `.ico`, installer
banner, favicon, and release art remain packaging polish rather than functional
setup dependencies. Do not add duplicate raster assets merely to fill these
slots.

## Copy Rules

- Use plain questions and concrete consequences.
- Call the current managed runtime a **verified release** or **managed runtime**;
  do not describe it as a local source build.
- Describe Ollama as existing/user-managed unless the GUI gains a verified
  Ollama installer action.
- Never claim a feature was installed merely because a job was queued.
- Never display saved connection keys, API keys, or bearer tokens.
- Failed optional setup must say what remains usable and where retry lives.

## Acceptance

- Fresh store redirects to setup; an existing store does not.
- Keyboard-only navigation and radio selection work.
- Runtime choice persists before model selection can be skipped.
- Managed llama.cpp is the fresh-install Recommended default; Ollama is never
  selected implicitly.
- A compatible Ollama-library model can be scanned and explicitly imported
  from the setup model step.
- Cloud key stays write-only and connection check causes no motion.
- One explicit reviewed plan owns every selected local install; it is
  controller-gated, visible, cancellable, retryable, and leaves voice disabled.
- Setup can be abandoned and resumed without losing completed writes.
- Setup is re-runnable from Settings and completion returns to Chat.
- 1280x800 and 390x844 visual checks show no overlap, clipping, or horizontal
  overflow in the default and at least one custom theme.
- Reduced-motion mode has no repeating animation.

## Cross-References

- [GUI installer decision](gui-installer.md)
- [Installation automation](installation-automation.md)
- [Windows release packaging](windows-release-packaging.md)
- [UI design guidelines](ui-design-guidelines.md)
- [ADR 0011](decisions/0011-windows-installer-shell.md)
- [IMPLEMENTATION_PLAN Phase 16](../IMPLEMENTATION_PLAN.md)
