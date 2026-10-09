# Hosted model implementation and setup review — 2026-10-08

Branch: `codex/chatgpt-motion-planning`.
Baseline: alpha.57, `eb7f7c504e2b224866c9976271dcef932c90225d`.
Decision: [ADR 0039](decisions/0039-hosted-model-connections.md), proposed.

## Review status

Claude CLI 2.1.280 identified its model as `claude-opus-5-5`. Its source-based
setup consultation completed successfully and found returning-user overwrites,
unnecessary local downloads, stale role/readiness state, insufficient disk
gating, popup handling and install-retry problems. Those findings informed the
implementation and regression tests. The consultation did not run the app.

Initial tool-based implementation attempts exhausted their turn limits without
a final report. The subsequent full-source implementation review received HTTP
429: the Claude account session limit resets at 18:20 America/New_York on
October 8. The final independent pass is therefore pending. This is not a
Claude approval of the final diff, a security audit or release acceptance.

## Setup and settings behavior

Easy presents Local-only AI, ChatGPT and API / Cloud. OpenRouter appears only
in the API / Cloud branch. Account/key connection and catalog/model selection
precede a separate combining question. Keeping chat local sends technical
motion context to cloud Autopilot; the UI states that it does not interpret
conversation text. Otherwise the selected cloud connection handles chat and
its permitted motion. Advanced routing, Decisions and named role editing remain
in Settings/Custom rather than the ordinary Easy flow.

Account management and manual IDs are disclosures. Model catalogs load after
connection automatically; a real generation requires the explicit Check model
action. The UI distinguishes draft model choices from immediately saved
accounts/keys. Unchanged readiness can survive browser reload or editor switching;
model, endpoint, policy or credential changes invalidate it. A missing model
catalog does not revoke a valid generation proof for a manual model ID. App restart requires
another generation check. The backend rejects untested hosted setup completion.

Returning external llama.cpp/Ollama and custom role/context assignments are
preserved. Cloud-only submits no local runtime/model download. Assessment and
disk readiness gate Continue; no-install plans go directly to Finish. A failed
install retries the original backend plan, including private voice sample data,
without exposing that data in public status or rebuilding from Easy defaults.

Desktop and narrow-screen checks covered the three choices, disconnected
ChatGPT, connected account/model controls, the API-key branch and the combining
question. Saved draft keys can be queried by exact endpoint binding and removed
without first saving role assignments; the query never returns a secret, mutates
credential generation, invokes a provider or dispatches motion. The tested 1280 × 720 desktop places sign-in above the footer. At
390 × 844 and 320 × 568, document width matches viewport width; fields remain
inside the page and Stop stays reachable. Unselected descriptions collapse on
small screens. Keyboard Tab reaches the combining switch from sign-in. Pending,
expired, missing-permission and read-only account states have focused component
tests; those states were not recreated against the human's live account.

## Live authorization and text generation

The human performed sign-in. The first authorization attempt exposed an invalid
host identifier: the arbitrary `magichandy-...` form was rejected. Host IDs now
use stable UUID-v4 URNs, with migration limited to never-registered legacy hosts.
The successful retry reached a gray callback screen because the callback server
could close before the response drained. Graceful shutdown now joins/drains the
handler, and the confirmation page gives explicit success and a validated
loopback return link. Callback tests read the full success/failure HTML.

The protected account remained connected across review process restarts. The
authenticated catalog listed GPT-6.1-Sol and the model was explicitly selected.
The final app at `http://127.0.0.1:50214` passed
`scripts/check-review-llm.ps1` with its current controller ID and a real
`{"ready":true}` generation. The final chat path answered a neutral arithmetic request with “3 + 3 = 6.”
in 3,169 ms, one provider call, no malformed output, repair, fallback or motion
event. An earlier fresh-session run returned “The connection works. Motion
remains off.” in 2,991 ms before the final saved-key lookup change.
The visible motion source was Off and the simulator remained idle.

Earlier final-build retests produced incomplete and unavailable provider
outcomes. Both were held without fallback or motion. Their cause is not
established; they are retained as failure evidence rather than counted as
successful latency samples. A separate diagnostic of the same provider and
current app handler also completed with zero fake-transport commands. That
diagnostic is additional evidence, not a substitute for the final app request.
A probe without the Stop sequence was rejected before generation; the corrected
probe used the current backend sequence.

The local llama.cpp model `bonsai2-27b-ptq1-review` at loopback port 18182 also
passed a real readiness generation and text-only app chat earlier in the review.
It was offline during the final compatible-provider catalog check, which showed
the unavailable state and kept Continue disabled. A live mixed local/cloud run
remains dependent on an available local endpoint. Own-key OpenAI,
OpenRouter and Decisions billing paths have protocol fixtures; live credentials
for those vendors were not supplied. No Handy connection key was entered and
no real hardware was connected or commanded.

## Regression and security coverage

The final checks passed:

- Frontend typecheck, localization (2,856 keys in five locales), production
  build and 102 test files / 785 tests. The final layout-only adjustment also
  passed the focused setup suite and production build.
- `go test ./...`, `go test -race -timeout 20m ./...`, `go vet ./...`,
  `golangci-lint run ./...` with zero issues, and `CGO_ENABLED=0` production build.
- Existing architecture/import boundaries, motion/transport teardown and Stop
  gates, plus route-admission tests for the new hosted/setup mutations.
- PKCE/nonce/state and identity-claim validation, stable registration retry,
  account separation, refresh rotation/scope loss and credential-provider/endpoint
  boundaries. Scope loss cancels old token sources and invalidates readiness.
- OpenAI strict schema/domain conversion and Responses terminal completion;
  compatible/OpenRouter capability routing and terminal SSE handling. Refusal,
  quota, truncated, incomplete and malformed output cannot release partial motion.
- Hosted chat/history preservation, no local-runtime requirement, technical
  context allowlisting, independent request lanes, no local repair/fallback, and
  late-result rejection after Stop, settings/account/session/personalization
  changes, including cancellation-ignoring providers.
- Easy choice visibility, local/server preservation, explicit local/cloud roles,
  disk gating, real readiness, exact signature reuse/invalidation, failed plan
  retry, blocked popup and read-only controls.

The Windows race compiler was an isolated LLVM-MinGW 20261006 UCRT toolchain,
downloaded from its official release and verified against asset SHA-256
`317492c456aa27ee607a5919f1d2d38dcdc1112516a24d0bf4b00d078f52d17a`.
It is test tooling under ignored scratch storage; production remains CGO-disabled
and no runtime dependency or gate was changed.

## Shared-engine motion preview

Two neutral, stateless ChatGPT planning requests used the inert Lab comparison
endpoint. Each made one GPT-6.1-Sol request and produced a validated proposal
with 121 preview positions. Neither played, recorded motion or dispatched to a
transport. Both preserved requested speed 25 and the 0–100 outer range.

| Mode | Request | Generation smoke timing | Visual finding |
| --- | --- | ---: | --- |
| Creative v2 | Centered full-range strokes, even directional timing, no rebounds | 5,351 ms | Broad symmetric strokes; inherited variation still breathes the endpoints and timing, so individual strokes are not identical full-span repetitions. |
| Layered | Gentle evolving drift, variation 20, memory 8 cycles | 8,768 ms | The stroke envelope widens and narrows smoothly around the center within the outer range; bounded drift remains visible. |

The shared `motion-atlas` compiler exported both accepted proposals, and the
renderer produced an overview and two detailed plots. All were inspected.
Whole-percent wire points tracked the planned positions in the initial excerpts;
both reported zero knot velocity/acceleration jumps. Creative v2 showed the
larger speed outlier (peak 287.6 %/s versus Layered 163.9 %/s). Planned
acceleration/finite-segment jerk remain commanded estimates, not measured device
feedback or comfort limits. These are two generation smoke checks, not reactive
latency distributions, first-target/retarget acceptance or physical testing.
No new motion mode or motion character was introduced by this setup follow-up.

Local evidence, intentionally excluded from git, is retained at:

- `.scratch/opus-setup-consultation.json`, `.scratch/opus-source-review.json`.
- `.scratch/provider-go-current.log`, `.scratch/provider-race-current.log`,
  `.scratch/provider-frontend-final.log`, `.scratch/provider-final-chat.sse.txt`
  and `.scratch/provider-final-math-chat.sse.txt` and `.scratch/provider-final-v8-chat.sse.txt`.
- `.scratch/provider-motion-review.json`, `.scratch/provider-motion-trials.json`,
  `.scratch/provider-motion-atlas.json` and `.scratch/provider-motion-atlas/`.
- `.scratch/provider-budgets.json`, `.scratch/provider-runtime-budgets-v6.json`,
  `.scratch/provider-runtime-budgets-v7-idle.json`
  and final desktop/mobile setup screenshots.

Budgets, including the observed working-set increase, are in the
[scorecard](goal-scorecard.md). Credentials, personal account labels, runtime
databases, logs, traces, toolchains and generated review images are not committed.
