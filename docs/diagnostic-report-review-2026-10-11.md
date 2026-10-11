# Diagnostic report download review — 2026-10-11

Settings → Diagnostics now has **Download diagnostic report** and **Customize
report** in one group. The separate trace card is removed. View status summary
retains the existing plain-text Copy / Refresh actions. Options use graphite
surfaces, dividers and steel-azure switches; the chat select stacks on mobile.

## Inclusions

| Section | Default | Contents |
| --- | --- | --- |
| Status | Always | App/build/platform, current motion/mode, model status, voice workers, transport status/latency and media-tool status |
| Settings | On | Public settings/load provenance; freeform personalization is separated |
| Motion traces | On | Live trace and last retained stopped run; absence is noted |
| Chat | Failures only | Error metadata across chats; alternatively None or Full chat history |
| Failed model output | Off | Raw and repair responses from retained failures |
| Personas and lore | Off | Saved personas, descriptions, lore and default persona/custom-anatomy text; no portraits |
| Prompts | Off | Prompt sets, voice prompt/reference text and system prompts/schemas captured with included failures |
| Saved memories | Off | Enabled and disabled entries |

Full history includes all retained committed messages in saved and unsaved
sessions, without inheriting the UI's 200-message page limit. Existing retention
is 1,000 messages per session and 24 failures across chats. Pending, pruned or
deleted messages are unavailable. Captured prompts can contain conversation,
persona and memory text, as explained beside the option. The ZIP includes a
manifest and separate JSON sections; full conversations use indexed JSON Lines
files. See [ADR 0043](decisions/0043-selectable-diagnostic-reports.md).

## Verification

Backend tests exercise default exclusions, independent prompt/output choices,
credentials across all sections including escaped JSON forms, both live/stopped
traces, 233 messages plus a separate unsaved session, full message text, pending
output exclusion and absence of model/motion calls. Invalid selections or malformed
stored failures return errors without a partial ZIP. Cancellation, byte limits,
host authorization and independent Stop/status admission are covered.

Frontend tests cover defaults, choices surviving collapse, output disabled when
chat is excluded, busy/error/offline states and navigation cancellation. Full
Go tests and race tests, vet, lint, typecheck, localization, the pure-Go build,
and the frontend build and 850 tests pass. The route admission matrix explicitly
classifies the new endpoint as host-private.

Actual browser downloads were inspected: the default ZIP has five files / 5,067
bytes and no conversation; all options produce ten files / 12,504 bytes and all
three committed messages in the isolated review chat. The manifests match the
form. This new simulator has no stopped run, which the manifest records.

Desktop and 390 × 844 mobile views were inspected. Options and buttons remain
usable, there is no horizontal overflow, and Stop remains visible. The viewport
was restored, and customization is left expanded with default inclusions.

Ignored evidence under `.scratch/diagnostic-report/`: `default-report.zip`,
`custom-report.zip`, `desktop-collapsed.png`, `desktop-review.png`,
`mobile-expanded.png`, `mobile-options.png`, `measurements.json`, gate logs,
`review-chat.sse` and `review-llm.log`. No runtime report or credentials are
committed. See the [scorecard](goal-scorecard.md) for size/timing measurements.

## Review app

`http://127.0.0.1:50227/#/settings/diagnostics` serves current source with isolated
simulator data, motion/Autopilot off, and available local Ollama Gemma 12B. The
readiness probe passes. A real chat request returns a non-empty reply with one
provider call, no repair/fallback and no motion; 3,148 ms includes 2,797 ms of
model loading. User sessions are preserved. Model prompts, inference context
and motion behavior are unchanged by this feature.
