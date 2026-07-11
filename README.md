# MagicHandy

**MagicHandy is a free, open-source, local-first app that lets a local AI
control your [Handy](https://www.thehandy.com/).** You chat with an assistant —
or let it take over — and it moves your device in real time. Everything runs on
your own machine: your conversations, settings, and device key stay local. No
account, no third party in the middle, no tracking.

> **Status:** early and under active development. It already works — local chat
> drives real device motion — but it isn't packaged for one-click install yet
> and some features are still in progress. Expect rough edges, and see
> [what's coming](#roadmap-highlights).

## What it does

- **Chat that moves the device.** Talk to a local LLM (llama.cpp or Ollama); it
  replies *and* drives motion through one shared, safe motion engine.
- **Hands-free modes.** Freestyle keeps things going on its own; an LLM-driven
  Autopilot (letting the assistant change things up from the conversation) is on
  the way.
- **You stay in control.** Live speed / stroke / direction controls apply
  instantly, and an emergency **Stop** is always one click (or `Esc`) away on
  every screen.
- **Local and private.** A single lightweight app; your data lives in a local
  database on your computer, not a cloud.
- **Runs light.** It's a Go rewrite built for efficiency — the core idles in tens
  of megabytes, not hundreds.

## Requirements

- A **Handy** with firmware v4 and API v3 access.
- **Windows** is the primary platform today; Linux and macOS builds are
  best-effort.
- A **local LLM** for chat — [llama.cpp](https://github.com/ggml-org/llama.cpp)
  (recommended for NVIDIA GPUs) or [Ollama](https://ollama.com/). MagicHandy
  talks to these; it doesn't bundle a model, so you pick and download one you
  like.

Settings > Model lists models reported by Ollama and MagicHandy's managed GGUF
copies. You can import a standalone GGUF or point the app at an existing Ollama
models directory and copy a compatible model into the managed store. Imports
show progress and verify SHA-256; they never modify the Ollama library.

## Get started

**The easy way (Windows).** From the project folder, in PowerShell:

```powershell
.\install.ps1
```

The installer checks for [Go](https://go.dev/dl/), builds MagicHandy, sets up
your data folder, and can help you get a local LLM running. When it's done it
opens the app in your browser at <http://127.0.0.1:49717>.

**Prefer to do it by hand?** See [Build from source](#build-from-source).

One-click packaging, guided model download, and GPU/CUDA setup are being brought
up to the polish of the original StrokeGPT app — see the plan in
[docs/installation-automation.md](docs/installation-automation.md).

## Privacy and safety

- **Local-first.** Chat, memories, prompts, settings, patterns, programs,
  preference feedback, and model metadata live in a local SQLite database on
  your machine; managed model files stay in the app data model store. Your
  Handy connection key is a private credential —
  it is never shown back in the UI, logs, diagnostics, or exports.
- **Emergency Stop, always reachable.** It's on every screen, works even for a
  read-only second tab or when the backend hiccups, and stops the device even if
  a network call fails.
- **You set the limits.** Live controls apply immediately, and hands-free modes
  stay inside the speed/stroke limits you choose and stop the instant you say so.
- **Adults only.** MagicHandy controls an intimate device. Use it responsibly and
  at your own risk.

## Build from source

Requires [Go](https://go.dev/dl/) 1.25 or newer.

```powershell
go run ./cmd/magichandy
```

The app serves its UI at <http://127.0.0.1:49717>. The browser UI ships
prebuilt, so you don't need Node just to run it. Your data lives under your OS
config directory (`MagicHandy/magichandy.db`); pass `-data-dir .\.local-data` to
keep it somewhere else, or `-addr 127.0.0.1:PORT` to change the port.

To work on the UI itself (Vite + React + TypeScript, built at build time and
embedded in the binary), see [`web/`](web/) and
[ADR 0009](docs/decisions/0009-react-frontend.md).

## Roadmap highlights

MagicHandy is a ground-up Go rewrite of StrokeGPT-ReVibed. Working today: local
chat driving real motion (Cloud REST and browser Bluetooth), live controls,
Freestyle, long-term memory, editable prompt sets, and the new React UI. In
progress or planned: Autopilot, a pattern/program library and authoring, voice
in/out, guided setup and model management, and packaged releases. The full
picture is in [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md).

MagicHandy and [LSO (Local Stroke Orchestrator)](docs/lso-merge-integration.md)
are being combined into one project on this Go core.

## For contributors

Contributions are welcome, from people and AI coding tools alike.

- **Start here:** [AGENTS.md](AGENTS.md) — the shared standards for everyone. It
  separates what's non-negotiable (device safety, the pure-Go core, import
  boundaries, secret/data hygiene) from what's a guideline you apply with
  judgment.
- **The plan and architecture:** [IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md).
- Work happens on feature branches and merges to `main` by pull request with
  green CI and review.

Validate a change before you push:

```powershell
gofmt -w cmd internal web
go vet ./...
go test ./...
go test -race ./...          # needs a C compiler; CI also runs it on Linux
$env:CGO_ENABLED = "0"; go build ./cmd/magichandy
(cd web; npm ci; npm run typecheck; npm run test; npm run build)
```

### Docs

- [Contributing standards (humans and agents)](AGENTS.md)
- [Installation automation plan](docs/installation-automation.md)
- [MagicHandy + LSO integration plan](docs/lso-merge-integration.md)
- [MagicHandy + LSO merge alternatives](docs/lso-merge-alternatives.md)
- [Implementation plan](IMPLEMENTATION_PLAN.md)
- [Goals and guardrails](docs/goals-and-guardrails.md)
- [Goal scorecard](docs/goal-scorecard.md)
- [Motion and transport contract](docs/decisions/0002-motion-transport-contract.md)
- [Pattern library and import contracts](docs/pattern-library.md)
- [Frontend strategy](docs/decisions/0004-frontend-strategy.md) ·
  [React frontend migration](docs/decisions/0009-react-frontend.md)
- [SQLite persistence](docs/decisions/0008-sqlite-persistence.md)
- [UI design](docs/ui-design.md) ·
  [UI design guidelines](docs/ui-design-guidelines.md) ·
  [UI navigation redesign](docs/ui-navigation-redesign.md)
- [HSP v4 invariants](docs/hsp-v4-invariants.md)
- [Risk register](docs/risk-register.md) ·
  [Performance baseline](docs/perf-baseline.md)

## License

MagicHandy is licensed under the GNU General Public License v3.0 only. See
[LICENSE](LICENSE).
