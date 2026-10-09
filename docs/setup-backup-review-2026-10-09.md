# Hosted setup: optional local backup and novice UX review

## Behavior

Both Easy and Custom setup now follow hosted Chat configuration with a separate
Local backup (optional) step. New users explicitly choose to configure one or
continue without one. Reconfiguration preserves an already enabled backup.
Local-only chat, including local chat plus hosted Autopilot, skips this question.
The primary Chat connection remains selected while installing or configuring a
local model. Declining disables local retry and schedules no backup download.

The step supports the existing managed-model install/import workflow, Ollama,
and an external llama.cpp server. A local server-type change clears its old
model ID; Ollama selection must match discovered models. Refreshing a changed
Ollama address saves that local endpoint before discovering its models.
Readiness is a fixed, bounded text generation on the saved local model, without
chat history, hosted credentials, saved-route changes or motion dispatch.
Existing servers can be checked on the Backup step, and that check carries to
Finish. Managed downloads are checked after installation. A failed check leaves
an immediate Continue without a backup action at Finish, which disables retry
without repeating installation. Local checks are canceled by Stop and takeover.

## Opus 5.5 High consultation

The user requested this exact model and effort. Claude CLI ran
`claude-opus-5-5 --effort high`, with tools disabled and a supplied source bundle
covering setup, provider settings and routing. It completed successfully in
118,077 ms; response metadata confirmed `claude-opus-5-5`. The consultation was
source-based, not an observed novice usability study. No credentials, chat
history or account data were supplied.

Accepted corrections:

- Offer an escape from a failed backup check directly on Finish.
- Hide the duplicate retry checkbox and settings Save/Discard banner in Custom
  setup; use Continue-oriented save wording there.
- Clear invalid model selections on local provider changes, require discovered
  Ollama models, and explain disabled Continue states.
- Use a separate setup notice preference. Keep the refusal-only scope in the
  choice card so dismissing help cannot imply outage/usage-limit failover.
- Use plainer backup runtime labels: Install a local model, Use my Ollama,
  Use my own server. Keep the technical details available.
- Include selected voice requirements in the backup-step disk estimate. The
  estimate uses the larger of the chosen model download plus margin and the
  backend's recommended local-install requirement; backend per-component
  preflights remain authoritative.

The UI review also corrected the inner setup scroller retaining the previous
step's position and local endpoint inputs missing the normal text-field style.

Larger follow-ups recommended by Claude, not implemented in this change:

- Explain ChatGPT account versus API-key billing more directly and define
  Autopilot at its first mention.
- Consider a single routing-choice screen with a destination preview instead
  of combining toggles and separate roles. Preserve saved advanced routing
  until the user explicitly replaces it.
- Consolidate hosted readiness wording and rules between Easy, Custom and
  settings; move advanced named connections/Decisions/provider routing behind
  a clear Advanced disclosure.
- Supply provider recommendations from backend capability/latency evidence,
  not catalog order. API model recommendations need provider-specific evidence.
- Add one backend estimate for the exact whole install plan, including selected
  runtime packages and partial downloads. The current estimate is approximate.
- Test the terms backup/retry, ChatGPT account/API key and Autopilot with actual
  novice users; the consultation cannot establish comprehension on its own.

## Verification

Component tests cover hosted-only skip, existing local servers, managed backup
installation preserving hosted Chat, Custom reconfiguration, clearing stale
local IDs, failure recovery at Finish and reusing an early local check. The
existing local-only setup suite remains green. Backend tests check local-only
routing, strict readiness output, no motion, and in-flight Stop cancellation.
The route has an explicit host-only admission policy. Global Stop also cancels
the local scheduler lane so non-chat readiness/warmup work is stopped on
installations without account sessions; no motion path or prompt contract changed.

Linux CI also exposed a missing G304 provenance annotation on the existing
credential-store lock path. Its sole caller uses the host-owned credential-store
path plus `.lock`, and Windows already documents that same invariant. The Unix
annotation now matches it; lint rules were not disabled or downgraded.

Runtime logs, source-review input/output and screenshots stay under ignored
`.scratch/`. No dependency was added and no physical device was connected.

The final frontend suite passed 104 files / 803 tests, with TypeScript and all
five localization catalogs checked. The final simulator review is at
`http://127.0.0.1:50219/#/setup/reconfigure`; the older review remains running.
`check-review-llm.ps1` passed against ChatGPT / GPT-6 Sol. A text-only production
chat returned `2 + 5 = 7.` in 5807 ms, one provider call, no repair, fallback or
motion. This individual latency is a readiness observation, not a performance
benchmark. The installed Ollama Gemma backup also generated a valid readiness
response through the setup button, without replacing hosted Chat.

Final Go checks passed: `go test ./...`, `go test -race -timeout 20m ./...`,
`go vet ./...`, Windows and Linux-targeted `golangci-lint run ./...`, and the
`CGO_ENABLED=0` shipping build. The final UI review verified the question starts
at the top of its scroller and a successful early local check is reused at
Finish. The review tab is left on Local backup (optional), with the existing
Ollama model selected and its successful check retained.
