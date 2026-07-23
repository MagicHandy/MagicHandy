# Clean Code — MagicHandy

## Change discipline

- Read the feature doc in `docs/tasks/features/` before coding
- One logical change per commit; message format: `feat(TICKET): description` or `fix(TICKET): description` (English)
- Branch name = ticket id (e.g. `BEESCLO-18927`)
- No credentials, connection keys, or local paths with secrets in commits

## Readability

- Prefer clear names over comments: `GenerateStrokeWaypointsFromPosition` over `genWP`
- Early returns over deep nesting in handlers
- Extract magic numbers to named constants when they encode protocol timing (see `procedural-chat-motion-analysis.md` §17)
- Delete dead code when replacing a path (e.g. legacy `GenerateChaoticWaypoints` — see feature 001)

## DRY without over-abstraction

- Reuse existing helpers: `writeError`, `decodeJSON`, `normalizeTipoBatida`, transport interfaces
- Do not add one-line wrapper packages
- Shared motion math stays in `internal/motion/`; do not duplicate waypoint logic in `httpapi`

## Documentation in code

- Update `docs/context.md` or an ADR only when architecture changes
- Update feature TRACKER when completing tasks
- Deep behavioral docs (e.g. procedural motion) belong in `docs/`, not long comment blocks in handlers

## Review checklist

- [ ] Import boundaries respected
- [ ] `go test ./...` and `go test -race ./...` pass
- [ ] Real-device session with sync analysis (Rule 04) for motion/transport/chat changes
- [ ] Frontend `npm run test` pass if UI touched
- [ ] No secrets in diff
- [ ] HSP invariants preserved if transport/motion touched
- [ ] TRACKER updated
