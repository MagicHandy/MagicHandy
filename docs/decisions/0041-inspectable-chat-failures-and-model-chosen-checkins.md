# ADR 0041: Inspectable chat failures and model-chosen check-ins

Status: Accepted, 2026-10-11; included in the alpha.61 release candidate.

## Context

The user report combined invalid Creative v2 envelopes, uncertainty about saved
prompt sets, missing failed-response evidence, and repetitive autonomous speech.
An experimental Lab prompt is not the live chat contract: it has different
lifecycle authority, reasoning settings, and personalization. The old inspector
also omitted the action instructions appended by the live service.

## Decision

- Compose live continuous-chat examples, lifecycle instructions and the response
  schema through shared functions used by both chat and its inspector. Keep the
  model's action explicit; missing actions never acquire start permission.
- Retain the latest 24 failed interactive requests across sessions in a separate
  SQLite table (schema 31). Each record has bounded metadata and private content.
  Redact configured credentials before truncation and storage, and again on read.
  Summary reads omit model output, prompt and schema; those require an explicit
  content request. Records never enter chat history, memory, mood or TTS. Clearing
  a chat clears its failures; deleting its session cascades. No success-path write.
- Offer a host-only production-configuration preview. It uses saved provider,
  persona, history, reasoning and the production service, under the normal model
  queue and Stop cancellation. It cannot dispatch motion or publish dialogue.
- Distinguish editing a prompt set from assigning it. Editor selection is only
  navigation state; the backend remains authoritative for the effective default
  and persona override. Display custom names rather than storage IDs.
- Let the speech model supply a short content intent before its reply. A literal
  empty reply, with a supplied intent, later timing and no motion, explicitly
  chooses a quiet check-in. The application does not interpret the words of the
  intent. A quiet turn schedules the next normal bounded opportunity and does not
  enter speech publication or wait for a playback acknowledgment.

No duplicate-word filter, phrase blacklist, similarity threshold, canned wording,
or language-triggered retry is added. Structural validation remains mandatory.
The existing single repair for malformed local output is unchanged. The separate
continuous-motion speech-authority contract retains its edits/reply envelope;
its prompt requires the non-empty reply that this contract accepts, while
the new quiet result is supported by the independent speech contract, including
the default speech-without-motion configuration.

## Consequences

Diagnostics improve reproducibility without weakening motion admission. A
preview checks the production service configuration, not dispatch side effects
or the whole interactive endpoint's special Stop/refusal-fallback orchestration.
Prompt sets still cannot override the code-owned JSON contract. Raw diagnostic
content may contain private personalization or echoed conversation, so it stays
collapsed and explicitly included when copied.

Language quality remains model-dependent. The selected prompt reduced idle
speech on Gemma 12B while preserving an invited conversation; Vanessa continued
repeating status. Stronger silence prompts and a wording filter were rejected.
See [the review and limitations](../user-report-review-2026-10-11.md).

## October 23 amendment: hosted response correction evidence

[ADR 0053](0053-hosted-chat-response-repair.md) adds bounded automatic and manual
repair for eligible hosted chat JSON. The same failure table also retains the
initial rejected output when its replacement succeeds; ordinary valid replies
still create no failure record. Metadata includes the original canonical user
sequence and repair outcome, while private content includes initial validator
feedback and replacement output. Redaction, retention and administrator-only
content access remain unchanged. The optional live Repair response action is
an expiring server-owned continuation, never a persistent replayable command.
The production-context preview follows the saved hosted repair setting, with
no motion, speech or conversation publication.
