# Alpha.85 integration review — 2026-11-12

## Scope and evidence

Reviewed Claude's uncommitted `claude/scene-motion-toggle` and
`claude/vacuglide-support-plan` work against alpha.84 (`0af9655d`). Imported
source snapshots into `codex/release-alpha85`, rebuilt the one shipping UI,
and left the original worktrees untouched. Their source hashes still match
the captured inputs. No dependencies or native workers were added.

The review accepts the optional Advanced scene-motion setting and experimental
Autoblow Ultra/VacuGlide support after the corrections below. Device support
has simulation evidence only. Cloud-only Stop is the explicitly accepted
transport limitation recorded in ADR 0069, not a successful offline Stop.

## Blocking findings corrected

| Finding | Correction and regression coverage |
| --- | --- |
| HTTP redirects could forward the device-token header outside the selected server; event URLs also contain the token. | Clone the supplied HTTP client and refuse redirects for lookup, commands and SSE. Redact raw/encoded tokens from provider messages before truncation and from complete diagnostic exports. |
| A queued valve pulse could open after Stop because its generation was dropped before dispatch. | Carry the generation and admitted client through the whole pulse. Stop admission invalidates it synchronously. Serialize pulses so concurrent requests cannot pass the net-step limit together. |
| A lost valve-open acknowledgement skipped closure; a failed motor Stop skipped valve cleanup. | Treat an attempted open as possibly applied, always close on the same client, and attempt both valve closures despite motor Stop failure. Report unconfirmed operations. Both valve directions require controller/admin and the current Stop sequence. |
| Reconnection reused the previous playing timeline. | Reset timeline, origin and smoothing on connection. Bind session commands to their original client and retire the connection before replacement. Reconnecting never joins an existing run. |
| A steady motor could outlive repeated generic health failures. | Count consecutive failed reads, attempt Stop, retire the connection and let the existing linked failure policy act. Explicit device fault modes also fail readiness. |
| Delayed Stop/test cleanup could target a newer playback generation. | Capture the original connection and generation; join cleanup workers during teardown and reject stale completion. An uncertain test-start reply still attempts Stop. |
| ChatGPT Reconnect was disabled when the host PC used the public address; a LAN-only listener had no usable loopback workaround. | Initiate through the configured public origin using existing authenticated-administrator/controller admission. Keep the normal network/TLS/browser-origin gates and host-loopback OAuth callback. Explain host-browser completion; see ADR 0070 and the integration correction below. |
| Account reconnection was locked by an unrelated pending catalog/model check, and old results could reach the new account. | Keep account controls available during discovery/checking; ignore stale generation/signature results and rediscover after account changes. |

The first five transport/client regressions failed against the imported code
before correction. The combined tests additionally cover linked Stop-all and
continue-healthy policies, idle reconnect, uncertain test start, delayed Stop,
valve limits, token exports, and host/proxy identity. No deterministic language
classification was added.

## Motion and model assessment

`TestAutoblowMotionReview` compiles six shared-engine patterns at 5, 40 and
100% with both owners' actual sampling capabilities: 36 retained cases. All
compiled within bounds with ordered point timestamps. Both overview sheets,
the fast short/compound and directional details, and the VacuGlide motor
overview were inspected. The sparse Ultra input keeps endpoints and counted
reversals; fast short strokes become nearly triangular between transmitted
turns. The device's own go-to trajectory and cloud jitter remain unmeasured.

VacuGlide follows average absolute travel, so short strokes limited by the
shared fitter and strongly asymmetric patterns need not reach the motor's
maximum at a 100% requested pace. The motor plots show the intended smoothing
and stall floor, and no output outside covered time. This is a reference
mapping, not evidence of equal physical pace or shape. The UI now says
**Reference pace**, and labels both new devices experimental. The optional
valve follower remains off by default and needs physical tuning.

Advanced scene motion remains on by default. Off removes only that guidance;
Advanced explicit Stop and existing motion validation remain. Simple and
Intermediate prompts retain their existing contracts. Claude's retained
on/off latency observations are documented in ADR 0068 with their output and
provider-variance limitations; no new causal latency claim is made here.

## Validation

- Full `go test ./...`, full Windows `go test -race -timeout 20m ./...`,
  `go vet ./...`, and `golangci-lint` (zero issues).
- Frontend typecheck, five-locale audit, **1,108 tests**, and canonical build.
- Pure-Go production build with `CGO_ENABLED=0`; no new runtime dependency.
- Inert 36-case motion review above; all results retained under
  `.scratch/alpha85-review`, including numerical JSON and 36 detailed plots.
- Current-source isolated app at `http://127.0.0.1:50359`, simulated devices,
  available Ollama Gemma 12B at Intermediate/32K, no copied credentials.
  `scripts/check-review-llm.ps1` completed real generation. A text-only request
  through Chat returned a complete 32-token reply in 601 ms (144 ms to first
  token), without repair/fallback or motion. This is one readiness check,
  not a latency benchmark.
- Source UI: adding a device leaves it disconnected/excluded; simulated connection,
  motor Test, Stop and both valve directions work. Reference pace and the
  experimental note render correctly. Advanced scene motion appears only at
  the applicable capability, and Discard restores the saved Intermediate
  configuration. The initial socket-derived sign-in link was superseded in
  integration; stale catalog/check completion tests remain.

The installed application and its authentication were not modified. No real
device was connected or commanded. Real OpenAI account reauthorization was
not completed; host admission was tested against a local OAuth issuer fixture.
PR/main/release CI and publication are separate gates and must pass before
tagging or reporting this update published.

## Updates integration corrections — 2026-11-12

CI found reachable [GO-2026-6859](https://pkg.go.dev/vuln/GO-2026-6859), an
HTTP/1 server crash in Go's `net/http` MaxBytesReader path. Updates raised the
release toolchain to Go 1.26.11 and the source installer's patch boundaries
to 1.26.11 / 1.27.4, matching the official advisory. The initial failed CI
logs remain retained; no scanner or hard gate is disabled. Earlier validation
and budget figures above describe the original Go 1.26.10 source handoff.

A further deterministic reproduction confirmed that both pacers could
overwrite a health-marked stale state while asynchronous teardown waited,
then send one more go-to or motor-speed command. Both pacers now refuse stale
connections before observing frame coverage. The regression retains both
original failures and checks that neither state nor output revives. This
does not alter the shared sampler, mapping or accepted cloud-only Stop limit.

Independent review also reproduced an old session re-anchoring fresh playback
when reconnect occurred between its identity check and clock update. Both
owners now check the admitted client under the clock-update lock. A gated
simulator regression reproduces that interleaving for both devices and
retains the original failures.

Only alpha.85 joins the unsigned public allowlist; alpha.65 stays excluded
and alpha.86 remains rejected. Final independent review, patched-toolchain
budgets, all PR/main/tag gates and actual public-download verification remain
required. The user-started 50359 review app is preserved; a patched-toolchain
review process must run in parallel with separate isolated data.

The socket-IP sign-in link was rejected during middleware review: it can fail
the configured public Host policy and certificate validation. ADR 0070 keeps
that policy and certificate, permits authenticated controlling administrators
to initiate through the configured public origin, and retains the loopback
callback/state/nonce/PKCE checks. Full-handler admission tests exercise all
gates, including a NAT-like peer; UI explains completion on the MagicHandy PC.

Updates reproduced the original public-address rejection through the complete
handler stack, then passed the new admission suite and OAuth fixture checks.
Patched-toolchain focused race tests, frontend typecheck/localization and all
**1,107** frontend tests pass. Both complete installer suites pass sequentially
on PowerShell 5.1/7, including the 1.26.11 / 1.27.4 boundaries and future
alpha.86 rejection. Same-toolchain alpha.84/final stripped cores measure
24,494,080 / 24,877,056 B; final UI is 2,938,259 B across nine files. The
scorecard records gzip detail and separates the old-toolchain source figures.

A parallel patched review app runs at `http://127.0.0.1:50360` with fresh
isolated data, motion simulation and the same available local Gemma model.
Another real generation passed. The source app on 50359 remains running.
Final browser, text-only chat and publication checks remain required.

Updates independently reran the 36-case shared-engine atlas with Go 1.26.11,
rendered all 36 detailed figures and both overview sheets, and inspected the
fast short/compound/directional details plus all 18 motor curves. A separate
numeric audit confirms finite values, bounded position/motor commands and
ordered timestamps; the complete JSON equals the source handoff output.
The sparse wire and asymmetric/short-stroke mapping limitations above remain.
No rejected model selection or earlier regression failure was removed.

Final integration `go test ./...`, `go vet ./...` and lint (zero issues) pass.
The browser review is being completed separately against the exact running
integration build. Its final chat evidence and all exact-head CI/publication
checks must be recorded on the integration PR before release handoff.

## Remaining hardware acceptance

See ADR 0069 and the device review for physical Ultra/VacuGlide timing,
turns, valve effects, mode-button behavior and network-loss Stop latency.
ChatGPT sign-in must complete on the MagicHandy PC. An administrator on another
PC can initiate a pending attempt but cannot finish its host-loopback callback
there. Review used a local issuer fixture, not a real account reauthorization.
