# MagicHandy

**A free, open-source, local-first app that lets a local AI control your
[Handy](https://www.thehandy.com/).** Chat with an assistant to drive motion,
or let Freestyle run hands-free. Conversations, settings, and credentials stay
on your machine — no account, no tracking.

> **Status:** early and under active development. Local chat already drives
> real device motion, but there is no one-click install yet. Expect rough
> edges — see [what's coming](#roadmap).

## What it does

- **Chat that moves the device.** A local LLM (llama.cpp or Ollama) replies
  *and* drives motion through one shared, safe motion engine.
- **Hands-free modes.** Freestyle keeps things going on its own; LLM-driven
  Autopilot is on the way.
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

On Windows, from the project folder in PowerShell:

```powershell
.\install.ps1   # first-time setup — asks before installing anything
.\update.ps1    # later: update and rebuild, keeping your choices
```

The installer can start from a clean 64-bit Windows machine and provisions
only what your choices require. Flags, voice options, model imports, updater
behavior, and manual setup are all covered in the
**[Getting Started guide](docs/getting-started.md)**.

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
controls, Freestyle, long-term memory, editable prompt sets, a pattern/program
library, voice providers with push-to-talk, and model management. Planned:
Autopilot, guided setup, curated model downloads, and packaged releases. The
full picture is in [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md).

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
- [Installation automation plan](docs/installation-automation.md)
- [Motion and transport contract](docs/decisions/0002-motion-transport-contract.md) ·
  [HSP v4 invariants](docs/hsp-v4-invariants.md)
- [Pattern library and import contracts](docs/pattern-library.md)
- [LLM control surface — current state and ideas](docs/llm-control-surface.md)
- [Parity with StrokeGPT-ReVibed](docs/parity-with-stgpt-rv.md)
- [UI design](docs/ui-design.md) ·
  [UI design guidelines](docs/ui-design-guidelines.md)
- [MagicHandy + LSO integration plan](docs/lso-merge-integration.md)
- [Risk register](docs/risk-register.md) ·
  [Performance baseline](docs/perf-baseline.md)

## License

MagicHandy is licensed under the GNU General Public License v3.0 only. See
[LICENSE](LICENSE).
