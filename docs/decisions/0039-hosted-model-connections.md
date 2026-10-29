# ADR 0039: Hosted model connections and explicit context sharing

Date: 2026-10-08

Status: Proposed; implemented on main and released as a preview in alpha.58, pending final independent review.

## Context

Local chat remains useful for privacy and predictable cost. Some users want a
ChatGPT plan or a separately billed API model, and others want local chat with
cloud Autopilot. Onboarding must explain those choices without making every
user configure provider routing, credentials, context policy and role overrides.
Provider refusals, long latency and interrupted streams must not create a second
motion path or turn a failed request into an unrelated local decision.

## Decision

Settings keep the existing llama.cpp and Ollama fields and add bounded named
connections for ChatGPT, OpenAI Responses API, OpenRouter and compatible chat
completion endpoints. Chat and Autopilot can select connections independently.
An Autopilot assignment affects autonomous decisions; interactive chat uses its
conversation model for permitted motion. All accepted targets still pass through
the existing semantic validators, shared compiler, engine and transport interface.

Easy setup offers Local-only AI, ChatGPT and API / Cloud. A fresh installation
starts with local AI. OpenRouter appears only after selecting API / Cloud.
Cloud connection and model selection precede a separate optional combining
question. Enabling it keeps chat local and sends only technical motion context
to cloud Autopilot. The default for a newly selected cloud option uses that
connection for chat and motion. Cloud-only does not install a local runtime or
model. Returning users keep existing local servers and saved roles, including
custom combinations that Easy cannot accurately describe.

Model discovery follows a successful connection automatically. Generation is
an explicit Check model action. Setup completion requires a real successful
text generation for every selected hosted role. Readiness belongs to the exact
normalized connection and credential generation, is bounded and expires, and
does not authorize motion. Reloading the browser can reuse an unchanged proof;
restarting the app requires another check. Manual model IDs, account management,
provider capabilities, routing policy and Decisions are secondary disclosures
or Custom/Settings controls. Install retry uses the backend's recorded plan.

Conversation context preserves included messages as written, subject to normal
whole-message context budgets, with enabled persona and memory. Technical
context excludes conversation, persona, memory, custom pattern IDs and custom
anchor labels. It includes semantic motion, saved limits and recent numeric
state. The UI explains that technical Autopilot varies current motion without
interpreting the conversation. An October 9 amendment runs technical Autopilot in
Creative v2 and Layered through the same continuous planner as conversation
context, shown motion state only: the built-in behavior profile and utility
voice, without conversation, persona, memories, reaction style or spoken lines.
It may change pace and outer reach within saved limits. Recent speeds note which
were set through chat, without any words, and a chat-set pace is kept while it
remains in the three-minute speed history. Pattern, Decisions and Dynamic
technical planning are unchanged ([evidence](../technical-autopilot-review-2026-10-09.md)).
By default, refusals and malformed hosted output
are held without repair, rewriting, rerouting or a local fallback. The October 9
user-requested amendment permits an explicit `retry_refusal_locally` setting:
interactive chat may make one local generation after a typed provider refusal.
Other errors and valid replies with refusal wording do not trigger it. The local
attempt rebuilds the local contract, retains the original conversation, uses the
local scheduler lane and shared validation/publication path, and cannot repair
or recursively retry. It is visibly identified in chat and diagnostics. It does
not change the saved primary provider or Autopilot behavior. Stop and source
invalidation apply through both stages, including the gap between request lanes.

The October 23 [ADR 0053](0053-hosted-chat-response-repair.md) amendment lets
interactive hosted chat correct a structurally or semantically rejected JSON
response once using the original request, schema and exact validator feedback.
It defaults on; disabling it exposes a single-use action on the latest eligible
chat error. It never repairs provider refusals, truncation or transport errors,
and a repair cannot chain into the local refusal fallback. No rejected partial
motion is applied. Context, controller, Stop and atomic engine-plan guards
reject stale corrections. Local-only generation and Autopilot remain unchanged.

ChatGPT sign-in uses a host-only loopback OAuth flow, PKCE, nonce/state checks,
verified RS256 identity claims, and a stable UUID host identifier. Account and
workspace registrations remain distinct. Rotating credentials live in a host
private store with owner/SYSTEM permissions on Windows and mode 0600 on Unix,
outside app settings and exports. Refresh is serialized across processes.
Losing plan authorization invalidates existing readiness and request sources.
The callback server drains its confirmation response before closing.

### October 29 amendment: OpenRouter reasoning controls

OpenRouter connections can retain Provider default, disable reasoning, enable
provider-selected reasoning, or choose an advertised effort. Discovery reads the
catalogue's nested `reasoning` metadata, including mandatory reasoning and the
distinction between omitted, empty and null `supported_efforts`. An unsupported
saved choice fails before generation and remains recoverable in the editor.
Switching models retains only compatible choices. No effort is selected silently.

The compatible adapter sends OpenRouter's nested `reasoning` object, never the
OpenAI Responses or local llama.cpp reasoning fields. Capability discovery uses
the existing fifteen-minute provider cache. Changing reasoning invalidates the
provider and exact-connection readiness proof. Setup and Settings use the same
compact selector. A connection check allows 2,048 total output tokens when
OpenRouter reasoning may be active, because hidden reasoning shares that budget;
explicit Off keeps the existing 64-token check. This remains a bounded text-only
generation and does not authorize motion.

See the [implementation and model evaluation](../openrouter-reasoning-review-2026-10-29.md)
and [OpenRouter's reasoning contract](https://openrouter.ai/docs/guides/best-practices/reasoning-tokens).
ChatGPT/OpenAI reasoning behavior and saved local settings are unchanged.

### October 18 amendment: responsive authentication and Windows storage

Issuer discovery, code exchange, refresh/JWKS requests and revocation run outside
the manager mutex and the credential-file transaction lock. Refreshes share one
in-process operation and a separate per-registration file lock across processes.
The file transaction rechecks the active registration, persistent revision and
original credential snapshot before committing. Switching accounts or signing
out retires pending requests; a late response cannot restore their authority.
Welcome acknowledgments merge without discarding a valid rotating token. Close
cancels network work and waits for owned operations and callback listeners.

Sign-out clears local authority immediately and retains an in-memory copy only
for bounded best-effort revocation. An unconfirmed revocation is reported; a late
result cannot overwrite a newer sign-in. Registration/client mapping is retained.
These boundaries preserve [OpenAI's serialized refresh and credential storage
requirements](https://developers.openai.com/siwc/token-sharing-open-source/profiles-and-sessions).

On Windows, the complete credential payload is protected with current-user
[DPAPI](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata),
without machine-wide decryption or interactive prompts. Existing owner/SYSTEM
ACLs remain. Valid legacy JSON migrates under the file lock via a protected
temporary file and replacement; host/client IDs, account metadata and API-key
bindings survive. A versioned non-JSON envelope makes older binaries reject the
store rather than misread it as empty. No plaintext backup is written. Reading
unchanged credentials does not re-encrypt/rewrite the file. The existing pure-Go
`x/sys/windows` dependency supplies the platform calls; no dependency is added.

Unreadable, incompatible or undecryptable stores stay intact and expose only a
recovery flag and safe message. Setup and model settings offer an explicit,
administrator/controller-only reset with confirmation of its all-provider scope.
It rechecks unreadability under the same file lock, preventing a stale reset from
erasing newly repaired credentials. Reset removes the unreadable registrations
and model API keys, retires request sources, and creates a new host identity;
model/provider settings remain. It cannot revoke inaccessible remote tokens.

DPAPI protects copied files/backups from ordinary access outside the Windows
user context; it does not protect against code running as that user or a
compromised host. Windows migration is not backward compatible with old app
versions sharing the profile. macOS/Linux retain owner-only file protection;
native keyring integration is deferred. A Windows-sealed file on another
platform requires reset and reconnection. Local-only absent stores remain lazy.
See the [implementation evidence](../cloud-auth-hardening-review-2026-10-18.md).

OpenAI API and Decisions keys authorize separate billing; ChatGPT sign-in does
not enable either. Decisions chooses only opaque IDs for enabled, precompiled
library candidates or keep-current, through the existing library path.
OpenRouter defaults to denied data collection and disabled host fallbacks, asks
for compatible parameter support and avoids automatic transforms. Metadata and
privacy routing requests do not guarantee content acceptance or retention.
Compatible endpoints send only capabilities the user declares. Amended October 9:
a compatible endpoint on this computer left on automatic output with nothing
declared, such as LM Studio, an MLX server or llama-server, gets the standard
non-strict `json_schema` form and the app's temperature, top_p and token limit;
a rejected request reports the capability error. Remote endpoints
require HTTPS; loopback endpoints may use HTTP. Inference does not follow redirects.

Request lanes are bounded and independent per connection. Stop cancels all
lanes. Account, provider, settings, session and personalization changes fence
late results, including a provider that ignores cancellation. A provider outage
does not remove the global Stop or acquire hardware control.

The October 8 latency follow-up keeps Creative v2 as the existing user-facing
mode. Additional reach guidance was first selected only for hosted models. An
October 9 amendment applies it to every Creative v2 contract, because it raised
local steering as well ([evidence](../creative-v2-shared-reach-guide-2026-10-09.md)).
An October 10 amendment omits it for an external llama.cpp server, which may
drop the response schema; without the schema the guide made local replies
malformed. One-call inference is unchanged. Reasoning
effort is a connection setting and part of readiness identity. ChatGPT exposes
account-advertised efforts; new choices prefer Low when supported. Empirical
guidance recommends GPT-6 Sol / Low, without claiming an unverified Fast tier.
The October 28 follow-up changes the recommendation and empty ChatGPT model
selection to GPT-6.1 Terra / Low after a 113-request-per-model comparison found
lower latency with the same intent-check score. Discovery still respects the
account catalog, preserves saved choices and falls back to the previous model
order when Terra is unavailable. See the
[Terra evaluation](../chatgpt-terra-evaluation-2026-10-28.md).
Hosted strict schemas wrap root action unions in a required proposal object
and add constant types, then unwrap strictly before the original domain parser.
Credential directories and Windows ACL support initialize on first use, so a
disconnected local-only app does not pay their startup cost. Authorization and
file protections still run inside the serialized storage transaction.

The October 9 settings follow-up projects saved Chat and Autopilot routes from
the backend runtime resolvers into the public settings snapshot. The UI renders
that projection independently of unsaved edits. An unavailable connection cannot
silently select a local route; assigned hosted roles require a nonempty model.
Settings may retain unassigned configurations before credential setup, while
setup completion still requires actual inference. Provider-specific editors and
direct Chat model choices replace the shared connection editor. A separately
selected hosted Autopilot defaults to technical context. See the
[module review and language evidence](../provider-settings-review-2026-10-09.md),
including the completed Opus 5.5 source consultation and its scope limits.

## Consequences and remaining evidence

No new Go or browser dependency is added. The canonical embedded frontend and
CGO-disabled production build remain. Hosted adapters and translated UI add
binary and bundle weight. The initial Windows working-set increase was traced
to eager credential initialization; the lazy follow-up restores matched local
startup measurements. The scorecard retains both checkpoints and the outlier,
without changing gates.

The review has real local and signed-in ChatGPT text generation, simulated
failure/admission coverage, a 247-case shared-engine atlas and passing production
chat/fake-transport runs with GPT-6 Sol and GPT-6.1 Sol. It does
not establish physical comfort, provider latency distributions, live billing
with every API vendor, WAN acceptance, or an external security audit. Claude
Opus 5.5 completed the setup consultation; its final full implementation pass
is still blocked by its account session limit. See the
[review evidence](../cloud-model-review-2026-10-08.md).
The [latency follow-up](../cloud-motion-latency-review-2026-10-08.md) records
rejected prototypes, local Gemma comparison and the limits of these samples.
