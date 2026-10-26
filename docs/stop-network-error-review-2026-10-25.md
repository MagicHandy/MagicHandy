# Stop and "network error" in signed-in chat (2026-10-25)

## Report

After pressing Stop and then sending another message, chat showed "network
error" with every response. The installed alpha.74 used a signed-in Edge
session over remote HTTPS, an Advanced hosted chat model and Layered motion.

## Evidence

Read-only inspection of the installed instance:

- Stop landed at 4:19:46 PM while a reply was still streaming. The retained
  failure for that turn records "controller changed or disconnected".
- Every later reply was generated and committed normally, and the access audit
  records each chat command as a successful HTTP 200.
- No stored message, diagnostic or failure contains "network error". The text
  is the browser's own: Chromium's stream reader throws `TypeError: network
  error` when a response body is reset.

## Causes

1. **Server.** Stop cancels every request bound to the controller.
   `trackSessionActivity` then set the canceled request's write deadline to
   the present. On HTTP/2 that resets the stream at once (`RST_STREAM
   INTERNAL_ERROR`), so chat's own "Chat canceled by Emergency Stop." event
   never reached the browser. Only signed-in sessions bind requests, so
   installations without accounts already showed the intended message.
2. **Browser.** The chat panel reacted to Stop only after the next state
   snapshot, which arrived after the reset, and the catch path displayed the
   raw browser text. That failure had no link to its request, so each history
   update re-appended it below the newest reply: every later response appeared
   to fail.

## Reproduction

An isolated alpha.74 build ran with sign-in, a self-signed HTTPS certificate,
simulated motion and a local Ollama model, driven by headless Edge over CDP.

- Stop pressed 60 ms into a reply: Edge reported `net::ERR_HTTP2_PROTOCOL_ERROR`
  for the chat stream, the reply read "network error", and that bubble moved
  below each of the next two replies.
- Stop pressed between replies: no error.

## Fix

- **Stop has its own cause.** Emergency Stop cancels bound work with
  `errChatEmergencyStop`; takeover, ownership loss and revocation keep
  "controller changed".
- **Bounded closing grace.** Like shutdown, a Stop-canceled request may finish
  its socket writes for up to two seconds, so the stream delivers its
  cancellation event and closes normally. The canceled work itself stops at
  once, and Stop's fencing of motion, speech and admission is unchanged.
  Ownership and session revocation still interrupt immediately
  ([ADR 0029](decisions/0029-session-bound-network-control.md)).
- **Reads get the same grace.** An immediate read deadline fails HTTP/1.1's
  background read, which cancels the connection's base context. Once the
  response closed cleanly, that keep-alive connection served the tab's next
  requests already canceled: session checks returned 503 for several seconds
  and the app showed "Core connection lost". Verification found this
  regression in the first version of the fix; the regression test reuses one
  connection after Stop.
- **The browser stops its own reply.** This tab's Stop cancels its reply as
  soon as the reply's first event names the request it answers, with a
  two-second fallback, so the notice reads "Chat canceled by Emergency Stop."
- **App wording.** A dropped connection shows the existing localized retry
  explanation instead of the browser's text.
- **Failures stay put.** Every live failure is anchored to its request, so
  later replies cannot move it.

## Verification

- **Go.** `TestStopClosesSignedInChatStreamWithItsReason` (signed-in, HTTP/2)
  fails on alpha.74 with `stream error: stream ID 1; INTERNAL_ERROR; received
  from peer` and passes now; its takeover case still resets the stream.
  `TestStopLeavesSignedInKeepAliveConnectionUsable` returns 503 with an
  immediate read deadline and 200 now. Unit tests cover the cancel cause and
  both deadlines. gofmt, vet, golangci-lint (0 issues), `go test ./...` and the
  CGO-free build pass; `-race` runs in CI.
- **Web.** Four new chat panel tests (dropped connection, this tab's Stop,
  Stop before the first event, a request that ends before it starts) all fail
  against the previous panel and pass now. The localization audit, 1,008 tests
  in 125 files and the typecheck pass.
- **Live, HTTP/2 and HTTP/1.1.** On the fix build, this tab's Stop, a Stop
  sent through the API (as from another device) and a Stop before the reply's
  first event each leave "Chat canceled by Emergency Stop." under its request
  through later replies. The API Stop stream now finishes normally. No API
  request returned a non-200 status, and the composer stayed usable.
- **Review app.** `check-review-llm.ps1` passed, and a text-only chat replied
  "A clear daytime sky appears blue." with one provider call, no
  repair/fallback and no motion.

## Limits

- A live notice is display state: a full reload or a new conversation clears
  it, as before.
- If a request ends before the server records it (no first event), its
  request and notice stay together but still follow later replies. That case
  predates this change.
- No hardware was commanded, and the installed app and its data were only read.
