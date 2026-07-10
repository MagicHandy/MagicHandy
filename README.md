# MagicHandy

MagicHandy is a Go-first rewrite of StrokeGPT-ReVibed. The current application
starts a local HTTP server, serves an embedded single-page UI, exposes
health/state/settings endpoints, writes structured logs, runs local LLM chat
through provider adapters, and sends chat motion intent through the motion
engine.

## Current Scope

Implemented:

- pure-Go module and application entrypoint
- embedded static assets from `uibuild/dist` (built from `frontend/`)
- `GET /healthz`, `GET /api/status`, `GET /api/state`, and settings API routes
- fake Handy transport contracts, safe transport diagnostics, and `GET /api/traces`
- Cloud REST HSP v4/API v3 transport code, request-shape tests, and invariant tests
- semantic motion engine with active retargeting, latency-aware buffer lead,
  phase-preserving same-pattern changes, low-jump cross-pattern handoff, and
  retarget trace export fields
- Phase 7 retarget validation runner for safe real-device trace exports
- local LLM provider layer with llama.cpp as the primary HTTP path and Ollama as
  the secondary path, including managed llama-server setup fields and explicit
  load/unload endpoints
- streaming chat endpoint with strict JSON response validation, one repair pass,
  malformed-response UI indication, prompt sets, and motion-engine dispatch
- user-managed long-term memory (`/api/memory`): add, enable/disable
  individually or globally, remove, clear; enabled memories are injected into
  the chat system prompt, and chat works identically with memory off
- editable prompt sets (`/api/prompt-sets`) with protected built-in templates
  (duplicate to edit); the motion JSON contract is appended by code and can
  never be edited out of a prompt
- explicit settings factory reset (`POST /api/settings/reset`) behind a
  double-confirm control in Settings > Diagnostics
- autonomous modes as motion-engine clients (`/api/modes`): Freestyle drives
  bounded arrangement segments through deterministic style scoring (gentle /
  balanced / intense, a quick setting), with every planner decision — seed,
  score table, segment — recorded as trace rows; chat keepalive restarts the
  last chat target only after transport recovery, never after a user stop or
  pause
- JSON structured logging
- graceful shutdown
- SQLite persistence (`magichandy.db`) for settings, user memories, and editable
  prompt sets, with schema migrations, legacy JSON import, and redacted API
  views
- minimal browser settings UI for server, device placeholders, motion defaults,
  and diagnostics verbosity
- baseline tests, race-test compatible packages, and goroutine leak-test
  harnesses for future motion/transport loops
- CI for formatting, `go vet`, `golangci-lint`, tests, race tests, and a
  `CGO_ENABLED=0` build

Not implemented yet (see the status table in
[IMPLEMENTATION_PLAN.md](IMPLEMENTATION_PLAN.md)):

- voice workers and providers (Phases 12-13)
- pattern library, authoring, and migration (Phases 14-15)
- release packaging (Phase 16)

## Requirements

- Go 1.25 or newer (tested locally with Go 1.26.4)
- No Python or CGO dependency is required for the core app; the runtime is a
  single Go binary with the browser UI embedded
- Node.js 20+ and npm are needed only to build the `frontend/` React UI
  (development/CI), never to run the compiled binary

## Run From Source

```powershell
go run ./cmd/magichandy
```

By default the app listens on `127.0.0.1:49717`.

Useful flags:

```powershell
go run ./cmd/magichandy -addr 127.0.0.1:49718
go run ./cmd/magichandy -data-dir .\.local-data
go run ./cmd/magichandy -log-level debug
go run ./cmd/magichandy -version
```

Settings, saved memories, and user prompt sets are stored in `magichandy.db`
under the resolved app data directory. By default this is the OS user config
directory plus `MagicHandy`; use `-data-dir` or `MAGICHANDY_DATA_DIR` for local
development and tests. Legacy `settings.json`, `memories.json`, and
`prompt_sets.json` files are imported once and renamed to `*.migrated`.

Health checks:

```powershell
Invoke-WebRequest http://127.0.0.1:49717/healthz
Invoke-WebRequest http://127.0.0.1:49717/api/status
Invoke-WebRequest http://127.0.0.1:49717/api/state
Invoke-WebRequest http://127.0.0.1:49717/api/settings
Invoke-WebRequest http://127.0.0.1:49717/api/llm/status
Invoke-WebRequest -Method POST http://127.0.0.1:49717/api/llm/load
Invoke-WebRequest -Method POST http://127.0.0.1:49717/api/llm/unload
Invoke-WebRequest http://127.0.0.1:49717/api/transport/diagnostics
Invoke-WebRequest http://127.0.0.1:49717/api/traces
```

`GET /api/settings` and `GET /api/state` return redacted settings. The Handy
connection key can be saved through `PUT /api/settings`, but it is not returned
by diagnostics or settings reads.

Chat uses the selected local LLM provider from settings. The default provider is
managed llama.cpp at `http://127.0.0.1:8080`: configure a `llama-server`
executable path and a GGUF model path, then load it through `/api/llm/load` or
the UI. External llama.cpp mode connects to an already-running OpenAI-compatible
server at the configured base URL. Ollama is available at
`http://127.0.0.1:11434` through Ollama's own native `/api/chat` endpoint (the
MagicHandy browser route is `POST /api/chat/stream`). The core does not
download models automatically and does not link libllama.

The Phase 7 real-device retarget workflow uses the dedicated validation command:

```powershell
$env:MAGICHANDY_HANDY_CONNECTION_KEY = "<private Handy connection key>"
go run ./cmd/retarget-validate -max-speed 35
Remove-Item Env:\MAGICHANDY_HANDY_CONNECTION_KEY
```

Trace exports are written under `traces/` by default. The public Handy API v3
application ID is bundled; the private connection key is not returned by
diagnostics, trace exports, or settings reads.

## Browser UI (React)

The browser UI is a Vite + React + TypeScript app under `frontend/`, built to
`frontend/dist` and copied into `uibuild/dist` for Go embed (`uibuild/embed.go`).
Use `scripts/start_stack.ps1` or `Iniciar-MagicHandy.bat` to build UI + binary together.

```powershell
cd frontend
npm ci
npm run build      # frontend/dist → copy to uibuild/dist before go build
npm run test       # Vitest component/safety tests
npm run typecheck
npm run dev        # Vite dev server on :5173 (proxies API to the Go app)
```

## Validate

```powershell
gofmt -w cmd internal uibuild
go vet ./...
go test ./...
go test -race ./...
$env:CGO_ENABLED = "0"; go build ./cmd/magichandy
(cd frontend; npm ci; npm run typecheck; npm run test; npm run build)
```

`go test -race` requires CGO and a local C compiler. CI runs the race test on
Ubuntu so the gate is enforced even when a Windows workstation does not have
MinGW/GCC installed.

`golangci-lint run` is part of CI. Local developers can install
`golangci-lint` to run the same static checks before pushing.

## Planning Docs

- [Implementation plan](IMPLEMENTATION_PLAN.md)
- [Goals and guardrails](docs/goals-and-guardrails.md)
- [Goal scorecard](docs/goal-scorecard.md)
- [Motion and transport contract](docs/decisions/0002-motion-transport-contract.md)
- [Frontend strategy](docs/decisions/0004-frontend-strategy.md)
- [React frontend migration](docs/decisions/0009-react-frontend.md)
- [React UI implementation handoff](docs/react-ui-implementation-handoff.md)
- [SQLite persistence (ADR 0008)](docs/decisions/0008-sqlite-persistence.md)
- [UI design](docs/ui-design.md)
- [UI navigation redesign (sidebar shell)](docs/ui-navigation-redesign.md)
- [UI design guidelines](docs/ui-design-guidelines.md)
- [Localization wording](docs/localization-wording.md)
- [Prompt localization strategy](docs/prompt-localization-strategy.md)
- [StrokeGPT-ReVibed prompt inventory](docs/stgpt-rv-prompt-inventory.md)
- [HSP v4 invariants](docs/hsp-v4-invariants.md)
- [Risk register](docs/risk-register.md)
- [Performance baseline](docs/perf-baseline.md)

## License

MagicHandy is licensed under the GNU General Public License v3.0 only. See
[LICENSE](LICENSE).
