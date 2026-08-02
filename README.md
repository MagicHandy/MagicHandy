# MagicHandy

**A free, open-source, local-first app that lets a local AI control your
[Handy](https://www.thehandy.com/).** Chat with an assistant to drive motion,
or let Freestyle run hands-free. Conversations, settings, and credentials stay
on your machine — no account, no tracking.

> **Status:** early and under active development. Local chat already drives
> real device motion. An unsigned Windows setup binary and portable ZIP can be
> built by CI, but no installer binary has been released yet. Expect rough
> edges — see [what's coming](#roadmap).

## What it does

- **Chat that moves the device.** A local LLM (llama.cpp or Ollama) replies
  *and* drives motion through one shared, safe motion engine.
- **Hands-free modes.** Freestyle provides deterministic motion; Chat
  Autopilot lets the selected local model curate motion over time.
- **You stay in control.** Live speed / stroke / direction controls apply
  instantly, and emergency **Stop** is one click (or `Esc`) on every screen.
- **Local-first and private by default.** App data lives in a local database;
  network providers (Handy Cloud, ElevenLabs) are explicit opt-ins, never
  hidden dependencies.
- **Runs light.** A pure-Go core that idles in tens of megabytes, not
  hundreds.

## Requirements

- A **Handy** with firmware v4 and API v3 access (for Handy Cloud or browser
  Bluetooth), or a user-run [Intiface Central](https://intiface.com/central/)
  server with a supported linear actuator.
- **Windows** is the primary platform today; Linux and macOS builds are
  best-effort.
- A **local LLM** for chat: a managed
  [llama.cpp](https://github.com/ggml-org/llama.cpp) runtime MagicHandy builds
  and owns, or an existing [Ollama](https://ollama.com/) install. Models are
  never bundled or downloaded at startup.

## Get started

On Windows, open PowerShell in the folder where you want MagicHandy and paste
this entire block. It needs only Windows PowerShell and internet access. The
bootstrap repairs WinGet and installs Git when they are missing, builds the
core, and opens the guided setup screen:

```powershell
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$bootstrap = Join-Path $env:TEMP 'MagicHandy-bootstrap.ps1'
Invoke-WebRequest -UseBasicParsing 'https://raw.githubusercontent.com/MagicHandy/MagicHandy/main/bootstrap.ps1' -OutFile $bootstrap
powershell.exe -NoProfile -ExecutionPolicy Bypass -File $bootstrap
```

The default PowerShell path installs only the dependencies needed to build the
pure-Go core. Device, model, runtime, and voice choices live in the same GUI
used later from Settings. Choosing managed llama.cpp explains and installs its
source compiler toolchain; choosing an existing Ollama install avoids that
runtime and compiler footprint. Parakeet and local TTS remain explicit,
separate GUI actions and install into app-owned data folders. Unattended flags
remain available for managed deployments. Release clean-machine acceptance is
still in progress. Flags, voice options, model imports, updater behavior, and
manual setup are covered in the
**[Getting Started guide](docs/getting-started.md)**.

For later updates, open PowerShell in the same `MagicHandy` folder and run
`.\update.ps1`. It updates the core without replaying old optional-install
choices, then can open guided setup when you want to change them.

Versioned Windows builds also check the project's latest stable GitHub Release
and place an update notice in the app. The check can be set to manual-only in
**Settings > General**. MagicHandy never downloads or executes an update in the
background; packaged updates remain an explicit reviewed over-install.

Prefer to build it yourself? `go run ./cmd/magichandy` (Go 1.25+) serves the
app at <http://127.0.0.1:49717> — no Node required. Details in the
[guide](docs/getting-started.md#build-and-run-by-hand).

## Privacy and safety

- **Local-first.** Chat, memories, prompts, settings, patterns, and model
  metadata live in a local SQLite database. Your Handy connection key is a
  private credential — never shown back in the UI, logs, diagnostics, or
  exports.
- **Emergency Stop, always reachable.** On every screen, even a read-only
  second tab. It cancels motion and planners immediately and reports honestly
  if a transport stop could not be confirmed.
- **You set the limits.** Hands-free modes stay inside the speed and stroke
  limits you choose and stop the instant you say so.
- **Adults only.** MagicHandy controls an intimate device. Use it responsibly
  and at your own risk.

## Roadmap

MagicHandy is a ground-up Go rewrite of StrokeGPT-ReVibed. Working from source
today: chat-driven motion (Handy Cloud, browser Bluetooth, Intiface), live
controls, Freestyle, Chat Autopilot, long-term memory, editable prompt sets, a
pattern/program library, voice providers with push-to-talk, and model
management. The guided setup flow and unsigned Windows packaging workflow are
implemented on the Phase 16 branch; curated model downloads, prebuilt managed
llama.cpp bundles, signing, and release acceptance remain. See
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) for the full roadmap.

MagicHandy and [LSO (Local Stroke Orchestrator)](docs/lso-merge-integration.md)
are being combined into one project on this Go core.

## Contributing

Contributions are welcome, from people and AI coding tools alike.

- **Start here:** [AGENTS.md](AGENTS.md) — the shared standards for everyone,
  from safety non-negotiables to style guidelines.
- **The plan:** [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md).
- **Validating a change:** commands in the
  [Getting Started guide](docs/getting-started.md#validate-a-change).
- Work happens on feature branches and merges to `main` by pull request with
  green CI and review.

### Docs

- [Getting Started guide](docs/getting-started.md)
- [Goals and guardrails](docs/goals-and-guardrails.md) ·
  [Goal scorecard](docs/goal-scorecard.md)
- [Installation automation plan](docs/installation-automation.md) ·
  [Setup wizard design](docs/setup-wizard-design.md) ·
  [Release checks and update handoff](docs/update-checks.md)
- [Motion and transport contract](docs/decisions/0002-motion-transport-contract.md) ·
  [HSP v4 invariants](docs/hsp-v4-invariants.md)
- [Pattern library and import contracts](docs/pattern-library.md)
- [LLM control surface — current state and ideas](docs/llm-control-surface.md)
- [Parity with StrokeGPT-ReVibed](docs/parity-with-stgpt-rv.md) ·
  [Feature ideas catalog](docs/feature-ideas.md)
- [Video library and synced playback design](docs/video-playback.md)
- [Persona page and long-context design](docs/persona-page.md)
- [UI design](docs/ui-design.md) ·
  [UI design guidelines](docs/ui-design-guidelines.md)
- [MagicHandy + LSO integration plan](docs/lso-merge-integration.md)
- [Risk register](docs/risk-register.md) ·
  [Performance baseline](docs/perf-baseline.md)

## License

MagicHandy is licensed under the GNU General Public License v3.0 only. See
[LICENSE](LICENSE).
