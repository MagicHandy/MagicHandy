# Chat response interruptions and stalled provider streams

The installed alpha.71 session contained three user turns and no assistant
replies. All three requests ended with cancellation before any reply text:
36.372 s, 34.188 s and 8.259 s. The configured request deadline was 120 s.
The controller reports staying on the chat tab. The saved evidence does not
identify the exact trigger, so this investigation does not attribute these
cancellations to navigation, user action, an idle timeout, or provider refusal.

The selected provider was ChatGPT with `gpt-6-sol`, Low reasoning and Layered
motion. Each request used one provider call; no reply or motion proposal was
accepted. Prompt sizes were about 13 KB against a 112 KB admission budget.
There was no recorded schema rejection, refusal, authentication failure, or
context-budget exhaustion. A later controller heartbeat expiry occurred
after these failures and does not explain their timing.

## Live provider checks

Tests used the installed configuration's existing credential source without
copying credentials into a review profile. App diagnostic previews execute no
motion and save no dialogue. The direct probes imported only the provider and
authentication packages, with no engine or transport access. Private prompts,
responses, profiles and raw reports remain outside git.

| Check | Result |
| --- | --- |
| Installed diagnostic preview of the failed compound motion request | Valid Layered response; no motion applied |
| Installed diagnostic preview asking only for an acknowledgement | 120 s timeout, no provider activity or reply text |
| Minimal structured readiness request, Low | Completed in 36.208 s; first body byte at 16.252 s |
| Minimal structured readiness request, None | 60.004 s timeout; HTTP 200 headers at 8.965 s, no body bytes |
| Replayed Layered provider request, Low | Completed in 44.902 s; first body byte at 17.172 s |

The direct probes retrieved credentials in 0–1 ms and established the HTTP
connection in 59–71 ms. Large delays came later. This small sample establishes
intermittent provider-path latency even for tiny requests; it is not a model
benchmark, proof of a service-wide outage, or proof of the precise cause of the
original browser cancellations. Disabling reasoning was not a reliable fix in
this sample. A standard-library HTTP/1.1 and HTTP/2 reproduction also confirmed
that the server's read timeout does not automatically cancel inference after
the request body has been read.

## Confirmed application defects and changes

1. The Responses stream parser discarded a body-read cancellation/deadline and
   returned a generic incomplete-response error. Preserve the safe context
   error identity so the existing HTTP layer can say the model took too long
   to reply. Other read failures stay redacted, and partial proposals still
   cannot escape before terminal completion.
2. A browser-side cancellation could silently delete the pending assistant
   bubble. Keep a warning with the known Stop, control-loss or settings-change
   explanation, or the generic interruption explanation when unknown. Clear
   uncommitted output/provenance; retain existing committed-reply and manual
   repair handling. Unmounting still suppresses late callbacks.
3. Failure evidence reported the local reasoning switch (`off`) for a hosted
   request that actually used Low. Report the selected ChatGPT/OpenAI
   connection's effort; an omitted effort is labeled `default`.

These feedback changes alter no cancellation trigger, request deadline,
routing, retry policy or model selection. They cannot make a stalled provider
finish and do not resolve the unidentified original cancellation trigger.
The user subsequently switched to GPT-6.1 Sol and confirmed replies resumed;
three recorded replies completed in 8.860, 11.893 and 7.388 seconds. The
resulting motion prompted a separate
[slow-motion review](layered-slow-turn-review-2026-10-23.md), whose proposed
prompt/sampling changes are documented separately and await physical acceptance.

## Validation

- Regression tests first reproduced loss of deadline/cancellation identity and
  the missing warning bubble. Parser tests also check partial-output rejection
  and redaction of unexpected private read-error detail.
- Hosted reasoning evidence tests cover ChatGPT/OpenAI with None, Low and the
  provider default, plus unchanged local-model evidence.
- UI lifecycle tests cover Stop, controller changes, observation changes,
  hidden-tab cancellation and unmounting. Existing repair tests stay green.
- Go provider, HTTP API and architecture suites; frontend typecheck, all
  967 tests in 122 files, and production build pass. `golangci-lint`, `go vet`
  and the pure-Go build pass. Linux race checks remain a CI responsibility.
- The isolated current-source review app completed a real local Gemma chat in
  3.864 s (3.519 s model load), one provider call, with a nonempty response,
  no repair/fallback and no motion. `check-review-llm.ps1` also passed against
  that exact app.
- Browser verification of a text-only simulator request followed by Emergency
  Stop leaves “Chat canceled by Emergency Stop.” visible and the composer
  usable. The installed instance was not restarted, reconfigured or taken
  over; no real hardware was commanded.

The affected binary and canonical UI size measurements are recorded in the
[goal scorecard](goal-scorecard.md).
