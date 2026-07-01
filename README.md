# MagicHandy

MagicHandy is a Go-first rewrite of StrokeGPT-ReVibed. The current application
starts a local HTTP server, serves an embedded single-page UI, exposes
health/state/settings endpoints, writes structured logs, and keeps the
architecture boundaries ready for later chat and voice work.

## Current Scope

Implemented:

- pure-Go module and application entrypoint
- embedded static assets from `web/`
- `GET /healthz`, `GET /api/status`, `GET /api/state`, and settings API routes
- fake Handy transport contracts, safe transport diagnostics, and `GET /api/traces`
- Cloud REST HSP v4/API v3 transport code, request-shape tests, and invariant tests
- semantic motion engine with active retargeting, latency-aware buffer lead,
  phase-preserving same-pattern changes, low-jump cross-pattern handoff, and
  retarget trace export fields
- Phase 7 retarget validation runner for safe real-device trace exports
- JSON structured logging
- graceful shutdown
- versioned JSON settings with defaults, migration hooks, redacted API views,
  and atomic saves under the app data directory
- minimal browser settings UI for server, device placeholders, motion defaults,
  and diagnostics verbosity
- baseline tests, race-test compatible packages, and goroutine leak-test
  harnesses for future motion/transport loops
- CI for formatting, `go vet`, `golangci-lint`, tests, race tests, and a
  `CGO_ENABLED=0` build

Not implemented yet:

- full motion-control HTTP/UI workflows
- local LLM chat
- voice workers

## Requirements

- Go 1.24 or newer
- No Node, Python, or CGO dependency is required for the core app

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

Settings are stored in `settings.json` under the resolved app data directory.
By default this is the OS user config directory plus `MagicHandy`; use
`-data-dir` or `MAGICHANDY_DATA_DIR` for local development and tests.

Health checks:

```powershell
Invoke-WebRequest http://127.0.0.1:49717/healthz
Invoke-WebRequest http://127.0.0.1:49717/api/status
Invoke-WebRequest http://127.0.0.1:49717/api/state
Invoke-WebRequest http://127.0.0.1:49717/api/settings
Invoke-WebRequest http://127.0.0.1:49717/api/transport/diagnostics
Invoke-WebRequest http://127.0.0.1:49717/api/traces
```

`GET /api/settings` and `GET /api/state` return redacted settings. The Handy
connection key can be saved through `PUT /api/settings`, but it is not returned
by diagnostics or settings reads.

The main app still exposes only low-level manual transport routes for Cloud REST
and browser Bluetooth. The Phase 7 real-device retarget workflow uses the
dedicated validation command:

```powershell
$env:MAGICHANDY_HANDY_CONNECTION_KEY = "<private Handy connection key>"
go run ./cmd/retarget-validate -max-speed 35
Remove-Item Env:\MAGICHANDY_HANDY_CONNECTION_KEY
```

Trace exports are written under `traces/` by default. The public Handy API v3
application ID is bundled; the private connection key is not returned by
diagnostics, trace exports, or settings reads.

## Validate

```powershell
gofmt -w cmd internal web
go vet ./...
go test ./...
go test -race ./...
$env:CGO_ENABLED = "0"; go build ./cmd/magichandy
```

`go test -race` requires CGO and a local C compiler. CI runs the race test on
Ubuntu so the gate is enforced even when a Windows workstation does not have
MinGW/GCC installed.

`golangci-lint run` is part of CI. Local developers can install
`golangci-lint` to run the same static checks before pushing.

## Planning Docs

- [Implementation plan](IMPLEMENTATION_PLAN.md)
- [Goals and guardrails](docs/goals-and-guardrails.md)
- [Motion and transport contract](docs/decisions/0002-motion-transport-contract.md)
- [Frontend strategy](docs/decisions/0004-frontend-strategy.md)
- [UI design](docs/ui-design.md)
- [HSP v4 invariants](docs/hsp-v4-invariants.md)
- [Risk register](docs/risk-register.md)
- [Performance baseline](docs/perf-baseline.md)

## License

MagicHandy is licensed under the GNU General Public License v3.0 only. See
[LICENSE](LICENSE).
