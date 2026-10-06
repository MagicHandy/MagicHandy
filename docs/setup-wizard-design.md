# Setup Wizard Design

[gui-installer.md](gui-installer.md) records the architecture: Inno Setup is a
thin Windows shell and the embedded app owns interactive setup at `#/setup`.
This document describes the implemented Easy and Custom setup paths and the
remaining work. The reference sketch is
[setup-wizard-sketch.svg](setup-wizard-sketch.svg); it is illustrative rather
than a second UI specification.

## Design Goals

1. **One decision owner.** Normal `install.ps1`, the setup EXE, and Settings all
   lead to the same embedded flow.
2. **Safe to skip.** Every optional step can be skipped and setup never
   commands motion or starts capture. Voice turns on only for a module the user
   chose on the Voice step, and only through its visible "turn on" choice.
3. **Honest cost.** Runtime and voice choices show why they are useful, their
   license, hardware constraint, and approximate disk impact before install.
4. **Recoverable.** Setup is re-runnable from Settings and interrupted jobs can
   be cancelled or retried without reinstalling the core.
5. **Backend authoritative.** Account/session state, settings, job state, model
   inventory, hardware detection, and connection results come from the backend.
6. **Credential narrowness.** If password protection is selected, the password
   goes directly from the in-app form to the local account API. The thin
   installer never sees or persists it.

## Easy And Custom Setup

The Welcome step offers two paths ([ADR 0037](decisions/0037-easy-setup.md)):

- **Easy setup** (default, recommended): four steps. Welcome, Your setup,
  Install, Finish. The backend assesses the computer
  (`assessment` in `GET /api/setup`) and reports met, partly met or
  not met for chat, voice output and voice input, with a reason code and the
  disk space each needs. It picks the first curated model the GPU holds, or
  keeps a ready model that is already selected. The page asks how explicit
  chat should be, whether to speak replies aloud, whether to talk instead of
  typing (Parakeet), and
  optionally the Handy connection key. **Install and continue** submits the
  same install plan as Custom setup. Access stays local-only, and chat is
  skipped when its requirements are not met.
  Spoken replies prefer Qwen3-TTS when its combined LLM/voice VRAM estimate fits
  on NVIDIA; otherwise Chatterbox is selected, on CPU if necessary. Both modules
  remain visible choices with performance and reference-voice explanations.
  Insufficient or unknown VRAM produces a warning and does not block selecting
  Qwen. Selecting Qwen opens a guided sample-and-transcript panel on the same
  page, or an explicit option to configure it later in Settings > Voice.
  Its card and panel both explain that a sample and exact transcript are required
  before it can speak. The estimate comes from the backend, including the actual selected
  store model; the browser does not recalculate it (ADR 0037 amendment).
- **Custom setup**: the seven steps below. Welcome, Access, Device, Chat AI,
  Voice, Install, Finish.

## Entry And Completion

A fresh data store has `ui.setup_completed=false`, so any normal route redirects
to `#/setup`. Settings documents created before this field existed are treated
as already configured and are not forced through onboarding after an update.
A completed store that returns on a stale `#/setup` browser tab is sent back to
Chat. The `-setup` flag and Settings > General's **Run setup again** action use
`#/setup/reconfigure`, which is the explicit opt-in to reopen the wizard.

Completion is an explicit backend write. Leaving halfway preserves each saved
step and keeps setup required. The normal app shell remains mounted throughout,
including the always-reachable Emergency Stop.

## Decision Tree

```text
1. Welcome
   - app language
   - chat reply language

2. Access
   - Only this computer: loopback with no sign-in (default)
   - Require an account and password: create the first local administrator
   - remote use (LAN + local, Public) stays folded behind "Use MagicHandy from a
     phone or another computer" and is always available in Settings > Access

3. Device
   - Handy Cloud REST + write-only connection key + non-motion check, with a
     hint that the key is shown in the Handy Onboarding app
   - Browser Bluetooth
   - Intiface Central
   - Handy model: Original / 2 Standard / 2 Pro (sets stroke length and speed)
   - skip

4. Chat AI
   - engine: managed verified llama.cpp (default, Recommended with an NVIDIA
     GPU) / existing Ollama service / external llama.cpp server / skip chat
   - managed model: a curated download rated against the detected GPU
     (ADR 0034), a model already in the store, or "add a model later"
   - import a GGUF or copy one from an Ollama library (folded)
   - Ollama model or external server model identifier

5. Voice (both optional)
   - output: none / Faster Qwen3-TTS / Chatterbox / external compatible server
     - Qwen3-TTS: NVIDIA required; preferred unless combined LLM/voice VRAM
       estimate is insufficient (a manual choice is still allowed with a warning)
       - configure now: choose WAV + enter its exact spoken words; backend
         checks format and length; install with that validated reference
       - set up voice later: install Qwen, leave spoken replies off
     - Chatterbox: included Emily voice; GPU when it fits, CPU fallback otherwise
   - input: install Parakeet or skip
   - turn voice on when installation finishes (included voice, or validated Qwen reference)

6. Install
   - review the selected local components as one plan
   - install sequentially with per-component state, progress, terminal output,
     Cancel, and Retry

7. Finish
   - data directory
   - selected runtime, model and voice state
   - local address and pre-motion reminder
```

The Phase 15 StrokeGPT-ReVibed importer is not implemented, so no migration
step or disabled placeholder is shown.

## Access Choice

The local no-login path stays selected by default. Choosing protected access
reveals username, password, and confirmation fields in the same graphite/steel
setup surface. A passphrase must contain at least 15 Unicode characters, with a
longer unique passphrase still recommended. Confirmation reports match or
mismatch as text and a compact icon while the user types, uses `aria-live` and
`aria-invalid`, and does not rely on color alone. Continue calls the one-time
loopback bootstrap endpoint, then clears both password fields.
Account existence durably enables the login wall for this process and future
launches. The bootstrap cookie exists only to authorize the remaining protected
wizard steps: on the first transition to completed setup, **Finish and sign in**
saves setup, revokes the current session, clears the cookie, and presents the
ordinary password screen—even if the page was reloaded after account creation.
Reopening an already completed setup shows status rather than a second bootstrap
form and does not sign out an administrator who entered through the normal login
screen.

Most installs stay on one computer, so the Access step opens on Local only with
the optional account checkbox. **Use MagicHandy from a phone or another
computer** reveals **Local only**, **LAN + local**, and **Public**; a saved remote
configuration opens them directly. Local only retains optional account protection. Remote choices create the first
administrator and keep the user on Access to finish the shared HTTPS guide.
LAN setup creates a local certificate and offers its public trust root for
download. Public setup detects the egress IPv4 and CA terms, accepts an optional
domain instead, and obtains/renews a public certificate after explicit consent.
The checklist states exact local and external TCP ports, firewall exclusions and
router forwarding requirements. Discovery is not labeled as inbound reachability.
Manual certificates and a trusted proxy remain advanced options.

Certificate preparation cannot change the app listener. The guide saves only
after preparation and backend validation succeed; Continue waits for that save.
Changes take effect on restart. No router/firewall rule or trusted root is installed
automatically. A remote browser cannot bootstrap the first account and instead
receives the login screen's local-setup guidance. See
[ADR 0030](decisions/0030-guided-https-certificates.md) and the
[deployment guide](self-hosted-https.md) for renewal and recovery.
The exact UX and future linked-session seam are specified in
[account-gui-design.md](account-gui-design.md).

## Chat AI Choice

The engine and its model are one step, so a chosen engine cannot be dropped by
skipping a separate model step. The current managed option installs
checksum-pinned official Windows bundles. It is the fresh-install default and
keeps the **Recommended** badge on NVIDIA hardware because it gives MagicHandy a
pinned, app-owned runner whose startup, model loading, diagnostics, and shutdown
are under application control. CPU downloads about 18 MiB. CUDA downloads about
615 MiB, installs about 1.1 GiB, and requires a compatible NVIDIA driver and GPU.
Neither option installs a compiler or CUDA Toolkit. The screen states those
costs before the user continues. Without an NVIDIA GPU the step starts on Skip
and states that CPU replies are too slow for live chat; the engines stay
selectable for users with a model server elsewhere.

For managed llama.cpp the step lists the curated catalog (ADR 0034) with each
model's size, measured graphics memory, fit for the detected GPU, license, and
source page. The default is preselected only when the GPU holds it or its memory
could not be read. A model already in the store, including a catalog digest that
was imported earlier, is reused instead of downloaded. **Add a model later**
installs only the engine and finishes with a pointer to Settings > Chat > Model.
GGUF import and read-only Ollama library scanning with an explicit copy remain
under **Import a model file instead**; a finished import is selected
automatically. Continue stays disabled until a model choice is ready, and the
step says why. The footer's **Skip for now** skips chat entirely.

Ollama is never preselected or marked Recommended. It is an explicit option for
an existing installation and can save the managed runtime footprint by
using the user's daemon and model library. External llama.cpp is similarly
user-owned.

## Voice Choice

Faster Qwen3-TTS is offered only when an NVIDIA GPU is detected and uses CUDA.
Chatterbox offers CPU and CUDA. Each module shows its code/model licenses,
approximate disk impact, and reference requirement. The server installation
can be configured for app auto-launch.

Chatterbox is marked **Ready after install** because its included voice needs
no reference. Qwen opens an inline guide for a local WAV and its exact transcript,
with an explicit **Set up voice later** option. Its choice card and guide both
explain that an audio sample and transcript are required before it can speak.
The backend checks the sample's format and duration before installation and
again before saving it; it does not verify the spoken words.

When Chatterbox, a configured Qwen reference, or Parakeet is chosen, **Turn voice
on when installation finishes** appears, checked by default. With it on, a
successful install turns voice on and enables spoken replies for Chatterbox or
Qwen with a validated reference; the settings transition starts the workers.
Deferring Qwen's reference keeps spoken replies off, even when voice input is
enabled. An external voice server and advanced provider tuning remain in
Settings > Voice.

## Installation Jobs

The Chat AI and Voice pages collect choices; they do not expose independent
build buttons. Continuing from Voice submits one reviewed plan to the backend.
The backend runs managed llama.cpp, the chosen catalog model download, local
TTS, and Parakeet sequentially through one queue. A downloaded model is selected
as the chat model once it is verified. A queued or running plan blocks another install and exposes bounded,
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
- The Access step defaults to local/no-login, can create exactly one initial
  administrator, never sends the password through installer-owned state, and
  does not enable LAN exposure. Finishing a protected first run revokes the
  temporary setup session and requires the new password through the login UI.
- Choosing "add a model later" keeps the managed runtime in the install plan.
- Managed llama.cpp is the fresh-install Recommended default on NVIDIA hardware;
  Ollama is never selected implicitly; chat starts skipped without NVIDIA.
- The recommended catalog model is preselected only when the detected GPU holds
  it, downloads inside the plan with resume, and is selected once verified.
- A compatible Ollama-library model can be scanned and explicitly imported
  from the Chat AI step.
- Cloud key stays write-only and connection check causes no motion; the Handy
  model choice is saved with the Device step.
- One explicit reviewed plan owns every selected local install; it is
  controller-gated, visible, cancellable, and retryable. Voice turns on only
  for a chosen module that needs no reference, and only when the visible
  "turn on" choice is kept.
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
- [ADR 0034: curated model downloads](decisions/0034-curated-model-downloads.md)
- [IMPLEMENTATION_PLAN Phase 16](../IMPLEMENTATION_PLAN.md)
