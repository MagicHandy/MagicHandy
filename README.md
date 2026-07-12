# MagicHandy

**MagicHandy is a free, open-source, local-first app that lets a local AI
control your [Handy](https://www.thehandy.com/).** You chat with an assistant,
or let it take over, and it moves your device in real time. Conversations,
settings, and credentials are stored locally. Browser Bluetooth and local
LLM/voice providers can keep processing on your machine; the optional Handy
Cloud and ElevenLabs providers send the data required by those selected
services. MagicHandy itself has no account or tracking.

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
- **Local-first and private by default.** App data lives in a local database;
  network providers are explicit choices rather than hidden dependencies.
- **Runs light.** It's a Go rewrite built for efficiency — the core idles in tens
  of megabytes, not hundreds.

## Requirements

- A **Handy** with firmware v4 and API v3 access.
- **Windows** is the primary platform today; Linux and macOS builds are
  best-effort.
- A **local LLM** for chat. MagicHandy can build and own a pinned
  [llama.cpp](https://github.com/ggml-org/llama.cpp) runtime (recommended when
  you want direct runner control), or use an existing [Ollama](https://ollama.com/)
  install without adding that runtime. Models are never bundled or downloaded
  at startup.

Settings > Model lists models reported by external llama.cpp/Ollama servers and
MagicHandy's managed GGUF copies. Managed llama.cpp resolves its runner and
selected model from app-owned inventories, never user-entered executable/model
paths. You can import a standalone GGUF or scan an existing Ollama models
directory and explicitly copy a compatible model. Imports show progress and
verify SHA-256; they never modify the Ollama library.

## Get started

**The easy way (Windows).** From the project folder, in PowerShell:

```powershell
.\install.ps1
```

The installer checks for [Go](https://go.dev/dl/), builds MagicHandy, sets up
your data folder, and offers to build the pinned managed llama.cpp runtime from
source. It explains the benefits and storage cost before asking: choose **No**
to keep using an existing Ollama installation without the extra runtime, or use
`-SkipLlamaBuild` in an unattended install. When setup is done it opens the app
at <http://127.0.0.1:49717>.

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
- **Emergency Stop, always reachable.** It's on every screen and available to a
  read-only second tab. During active motion it immediately cancels local motion
  and planners, marks the engine stopped, and attempts an explicit transport
  Stop; any transport or backend failure is surfaced because software cannot
  claim a physical stop was delivered after communication failed.
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

MagicHandy is a ground-up Go rewrite of StrokeGPT-ReVibed. Working from source
today: local chat driving real motion (Cloud REST and browser Bluetooth), live
controls, Freestyle, long-term memory, editable prompt sets, pattern/program
library and authoring, voice provider adapters and push-to-talk UI, model
management, and the React UI. Voice providers still need manual provisioning,
and real microphone compatibility with the managed Parakeet path remains to be
validated. Planned work includes Autopilot, Intiface, migration, guided setup,
curated downloads, and packaged releases. See
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md) for acceptance gaps as well as
implemented scope.

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
go fmt ./...
go vet ./...
go test ./...
go test -race ./...          # needs a C compiler; CI also runs it on Linux
$env:CGO_ENABLED = "0"; go build ./cmd/magichandy
npm --prefix web ci
npm --prefix web run typecheck
npm --prefix web run test
npm --prefix web run build
```

### Docs

- [Contributing standards (humans and agents)](AGENTS.md)
- [Installation automation plan](docs/installation-automation.md)
- [Windows installer architecture](docs/decisions/0011-windows-installer-shell.md)
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
