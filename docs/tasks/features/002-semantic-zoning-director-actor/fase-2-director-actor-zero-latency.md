# Phase 2 — Director & Actor (Zero Time-to-First-Movement)

## Objective

**Director & Actor pattern:** hardware reacts before dialogue completes. Director returns `LLMIntent` via fast constrained JSON call; Actor streams roleplay text aware of current physical state.

## Prerequisites

- Phase 1 resolver complete
- Read `internal/llm/provider.go`, existing `POST /api/chat/stream`
- llama.cpp grammar / JSON schema support in managed runner

## Latency budget

| Step | Target |
|------|--------|
| Director call (JSON only) | < 300ms local (model dependent) |
| Resolver + motion dispatch | < 50ms Go |
| First HSP batch to device | < 500ms TTFP total |
| Actor first token | starts after Director completes (sequential LLM) |

Note: local llama.cpp processes requests sequentially — Actor stream begins milliseconds after Director JSON resolves.

---

## Task 2.1 — AskDirector

**Action:**

1. Add `internal/chat/director.go`:
   ```go
   func AskDirector(ctx context.Context, provider llm.Provider, userMessage string, history []llm.Message) (semantic.LLMIntent, error)
   ```
2. Use minimal prompt + JSON schema / grammar restricting output to `LLMIntent` fields only.
3. Reuse provider HTTP client pool; no new connection per request.
4. Single repair pass on malformed JSON (mirror chat contract).

**Acceptance criteria:**

- Returns validated `LLMIntent` or error
- Unit test with scripted provider fixture
- Log `director_latency_ms` trace row

---

## Task 2.2 — AskActor

**Action:**

1. Add `internal/chat/actor.go`:
   ```go
   func AskActor(ctx context.Context, provider llm.Provider, userMessage string, history []llm.Message, intent semantic.LLMIntent, onToken func(string) error) error
   ```
2. Inject dynamic system prefix:
   ```go
   fmt.Sprintf("System: You are currently performing %s on the %s zone at intensity %d/10. Match dialogue pace to this physical state.",
       intent.Action, intent.Location, intent.Intensity)
   ```
3. Stream tokens via existing SSE writer helper.

**Acceptance criteria:**

- Actor prompt contains intent fields
- Stream test asserts token order
- Does not include stroke math in prompt

---

## Task 2.3 — Handler orchestration

**Action:** Refactor `handleChatStream` (or parallel path behind settings flag):

```
a) Receive user message
b) AskDirector (blocking)
c) Validate intent → ResolveMotionBounds → dispatch procedural motion
d) Emit SSE event: event:intent data:{...}
e) Start AskActor stream in same goroutine (sequential LLM) OR goroutine if dual-slot runner later
f) Emit SSE tokens; close with motion summary event
```

**Acceptance criteria:**

- SSE order: `intent` → `token`* → `motion` (applied flag)
- Existing hybrid path unchanged when director mode off

---

## Task 2.4 — Concurrency safety

**Action:**

1. Reuse `chatChaos.generation` token for motion dispatch during Actor stream
2. Mutex on `chatChaosRuntime` state published to `/api/state`
3. Document: UI reads intent snapshot from SSE, not racing with motion visualizer

**Acceptance criteria:**

- `go test -race ./internal/httpapi/...` passes
- No double-dispatch on rapid messages (450ms throttle preserved)

---

## Task 2.5 — SSE contract update

**Action:**

- New SSE event `intent` with `{action, location, intensity}`
- Frontend: optional display of current action (phase 3 UI task)
- Update Rule 01 or new Rule 06 when contract frozen

**Acceptance criteria:**

- `chat_test.go` covers event order
- Redacted settings unchanged

---

## Phase completion

- Director mode behind `settings.LLM.DirectorMode` or similar flag
- Device test deferred to phase 3
