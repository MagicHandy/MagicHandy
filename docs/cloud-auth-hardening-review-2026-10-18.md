# Cloud authentication follow-up — 2026-10-18

Scope: issues [#306](https://github.com/MagicHandy/MagicHandy/issues/306) and
[#307](https://github.com/MagicHandy/MagicHandy/issues/307), based on alpha.63
`5e02daa0`. Layered planning PR #328 is separate. No prompt, language rule,
model routing, motion compiler or transport change is included.

## Status responsiveness and request lifetime

Credential transactions only cover local storage. Discovery, refresh, code
exchange, identity verification and revocation release the manager mutex.
In-process waiters share a refresh; a separate registration lock serializes
refresh-token rotation across app processes. Every successful or terminal
refresh result rechecks authorization before committing. A persisted revision
also catches another instance switching away and back to the same account.

The stalled-issuer fixture holds a completed response until released, including
after cancellation, so stale-result checks cannot rely on network cooperation.
In one Windows run, Status completed in **0.500 ms** in the refreshing manager
and **0.497 ms** in another manager while the token response remained blocked.
The test bounds this at two seconds to tolerate CI scheduling; this is a fixture
measurement, not a production latency percentile.

Coverage includes stalled discovery, callback exchange and revocation; sign-out
and account switches during refresh; cross-manager rotation; independently
canceled waiters; welcome acknowledgment during refresh; and Close cancellation
and teardown. Existing scope-loss, identity, redirect and diagnostic-redaction
tests remain in place. The package retains its goleak TestMain.

## Credentials at rest and recovery

Windows encrypts all credentials owned by `openaiauth`: ChatGPT access/refresh/ID
tokens, account registrations, Decisions keys and bound model-provider API keys.
DPAPI uses the current Windows user, preserves owner/SYSTEM ACLs, and never
falls back to plaintext when encryption fails. Legacy plaintext migrates once;
the encrypted envelope is intentionally not parseable as the old JSON format.

Migration tests use the real Windows DPAPI API with synthetic credentials and
check that every binding/identity is preserved, secrets are absent from disk,
unchanged reads do not rewrite ciphertext, and temporary files are cleaned up.
Corrupt ciphertext, unsupported envelopes and invalid JSON preserve the file
until explicit recovery. Reset cannot erase a readable or newly repaired store.
Public status and diagnostics retain their existing secret boundaries.

The browser walkthrough used a new simulator profile and an intentionally
unreadable synthetic store. Settings displayed the recovery explanation, then
the all-provider confirmation. Reset restored the provider list without changing
chat routing or model definitions. Component coverage also exercises ChatGPT
setup and OpenRouter key entry after recovery, cancellation, failure and locked
clients. HTTP tests cover confirmation, stale reset, unchanged settings and the
reviewed host-administrator admission matrix.

Platform limits: macOS/Linux keep existing owner-only file protection; this
change does not add their native keyrings. DPAPI is not protection from malware
running as the same user. Old app versions cannot read the migrated Windows
store. Recovery after a move to another identity/computer may require provider
sign-in again; remote grants must be managed at their provider if needed.

## Validation and review build

Local Windows validation: full normal and race Go suites, go vet, golangci-lint,
targeted authentication/HTTP race tests, pure-Go Windows build, frontend typecheck,
five-locale audit, component suite and production bundle build. The Windows
packaging workflow now exercises the actual DPAPI/migration/refresh tests too;
Linux CI continues to exercise owner-only fallback and the full race suite.

The isolated current build at `http://127.0.0.1:50258/#/settings/chat/model` uses
local Ollama Gemma 4 12B QAT. `check-review-llm.ps1` completed a real generation.
A text-only request through the app returned a non-empty response in **324 ms**
(first token **129 ms**), without repair, fallback or motion. No existing user
credential was copied, migrated, reset or used for live OAuth testing. OAuth
network acceptance here uses the signed protocol fixture; this is not a new
live ChatGPT login/refresh claim. No real device was connected.

The [scorecard](goal-scorecard.md) records the binary/UI deltas. Screenshots and
raw logs remain in ignored `.scratch/cloud-auth-hardening/`; no runtime files or
credentials are committed. ADR 0039 remains Proposed pending independent review.
