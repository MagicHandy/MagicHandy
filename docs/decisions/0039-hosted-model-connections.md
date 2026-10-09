# ADR 0039: Hosted model connections and explicit context sharing

Date: 2026-10-08

Status: Proposed; implemented on the review branch, pending final independent review.

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
interpreting the conversation. By default, refusals and malformed hosted output
are held without repair, rewriting, rerouting or a local fallback. The October 9
user-requested amendment permits an explicit `retry_refusal_locally` setting:
interactive chat may make one local generation after a typed provider refusal.
Other errors and valid replies with refusal wording do not trigger it. The local
attempt rebuilds the local contract, retains the original conversation, uses the
local scheduler lane and shared validation/publication path, and cannot repair
or recursively retry. It is visibly identified in chat and diagnostics. It does
not change the saved primary provider or Autopilot behavior. Stop and source
invalidation apply through both stages, including the gap between request lanes.

ChatGPT sign-in uses a host-only loopback OAuth flow, PKCE, nonce/state checks,
verified RS256 identity claims, and a stable UUID host identifier. Account and
workspace registrations remain distinct. Rotating credentials live in a host
private store with owner/SYSTEM permissions on Windows and mode 0600 on Unix,
outside app settings and exports. Refresh is serialized across processes.
Losing plan authorization invalidates existing readiness and request sources.
The callback server drains its confirmation response before closing.

OpenAI API and Decisions keys authorize separate billing; ChatGPT sign-in does
not enable either. Decisions chooses only opaque IDs for enabled, precompiled
library candidates or keep-current, through the existing library path.
OpenRouter defaults to denied data collection and disabled host fallbacks, asks
for compatible parameter support and avoids automatic transforms. Metadata and
privacy routing requests do not guarantee content acceptance or retention.
Compatible endpoints send only capabilities the user declares. Remote endpoints
require HTTPS; loopback endpoints may use HTTP. Inference does not follow redirects.

Request lanes are bounded and independent per connection. Stop cancels all
lanes. Account, provider, settings, session and personalization changes fence
late results, including a provider that ignores cancellation. A provider outage
does not remove the global Stop or acquire hardware control.

The October 8 latency follow-up keeps Creative v2 as the existing user-facing
mode. Additional reach guidance is selected only for hosted models; local
production/Lab contracts and one-call inference remain unchanged. Reasoning
effort is a connection setting and part of readiness identity. ChatGPT exposes
account-advertised efforts; new choices prefer Low when supported. Empirical
guidance recommends GPT-6 Sol / Low, without claiming an unverified Fast tier.
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
