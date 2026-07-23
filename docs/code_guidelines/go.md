# Go Code Guidelines — MagicHandy

## Module and packages

- Module: `github.com/mapledaemon/MagicHandy`
- Application code only in `cmd/` and `internal/`
- Package name = last path segment, lowercase, no `util` or `common`
- One primary type or concern per file when possible; `file_size_test.go` enforces limits

## Naming

| Kind | Convention | Example |
|------|------------|---------|
| Exported types | PascalCase | `MotionCommand`, `TimedPoint` |
| Unexported | camelCase | `dispatchChatMotion`, `writeError` |
| Constants | PascalCase or camelCase for unexported | `MotionGenerationModeProcedural` |
| Test files | `*_test.go` in same package | `organic_engine_test.go` |
| HTTP handlers | `(s *Server) handleFoo` or `registerRoutes` pattern | `handleChatStream` |

## Imports

- Group: stdlib → third-party → `github.com/mapledaemon/MagicHandy/...`
- Run `gofmt`; CI fails on unformatted files
- **Never** violate import boundaries in `internal/architecture/import_boundaries_test.go`

## Errors

```go
// Domain: wrap with context
return fmt.Errorf("build session: %w", err)

// HTTP edge: map to status
writeError(w, http.StatusBadRequest, err)
```

- Do not log and return the same error without adding value
- Stable error strings for tests; avoid formatting user input into errors returned to API

## Concurrency

- Long-running loops (transport status, chat auto, motion playback) use `context.Context` for cancellation
- Protect shared state with `sync.Mutex` or channels; run `go test -race ./...` before merge
- Prefer `goleak` tests in transport and motion packages when adding goroutines

## JSON and API types

- Request/response structs in handler files or `internal/*/types.go`
- Use `decodeJSON` / `writeJSON` helpers from `httpapi`
- Redact secrets before serializing settings to API responses

## Testing

```powershell
go test ./internal/motion/... -run TestOrganic -v
go test -race ./internal/transport/...
```

- Table-driven tests for parsers (`chatauto/parse_test.go` is the reference style)
- Use `t.Parallel()` only when tests do not share mutable global state

## Comments

- Package comment on every `internal/*` package
- Comment non-obvious timing constants (e.g. `chatChaosDispatchMinInterval`, `bufferAheadMS`)
- Do not restate the function name in doc comments
