# Alpha.61 release review — 2026-10-11

The user authorized an update release after the work on
`codex/asynchronous-motion-review`. PR #317 integrates six feature/review commits
above alpha.60 and the version-specific packaging amendment. No published tag
is moved. Release assets must come from the merged, tagged main commit through
the existing quality, package, vulnerability, Defender and installer gates.

## Independent source review

Claude Opus 5.5 / High reviewed the complete source diff (about 8,360 lines
including tests), ADRs 0040–0043, release notes, the alpha.61 packaging amendment,
and surrounding scheduling, history, permissions and redaction code. It ran no
tests and did not inspect hardware. The primary agent verified the findings.

- **Superseded correction handoff:** a chat activity could advance the mode
  generation while a review was applying, leaving `chat.pending` set. Both
  cancellation and delayed-success variants reproduced before the fix. Pending
  state now has an independent owner generation; review cleanup always releases
  its own handoff and preserves a newer handoff. The regression exercises actual
  concurrent admission/unwinding and verifies that Autopilot plans again after
  a chat turn with no motion edit.
- **Hidden-tab backlog:** with 250 unseen retained messages, recovery skipped
  the first 50. The display window now applies only on initial/reset snapshots;
  incremental revision pages deliver all retained changes. Anchored pagination,
  pruning and byte limits remain intact.
- **Continuous speech contract:** motion-capable Creative v2 / Layered speech
  receives a prompt that requires its non-empty reply. The default independent
  speech prompt and quiet-result grammar remain unchanged. No wording filter,
  additional generation or new language rule interprets the model's reply.
- **Default report personalization:** failure metadata excludes persona names
  and IDs unless the Personas inclusion is selected. Regression coverage checks
  both exclusion and explicit inclusion alongside credential redaction.
- **Status documentation:** ADRs now identify the alpha.61 release candidate.

The suggested preservation of an original error after credential-store failure
was not adopted: an arbitrary provider error is not guaranteed to be a safe
constant. The existing generic diagnostic-unavailable message remains the
fail-closed choice when redaction cannot complete. This can hide a useful hint.
Long hosted context may select older relevant lore; system-temp export files are
removed normally but can remain after a process crash. These are documented
limits, not claims that redaction removes all personal content.

## Validation

The new handoff and backlog tests fail on the pre-fix source and pass with the
fixes. Focused chat, mode and HTTP report tests pass. Vet, lint and the pure-Go
build pass. Full Go/race and PR CI results are recorded in PR #317 before merge.
The existing motion atlas remains applicable: these review corrections change
handoff cleanup, prompt contract selection and data projection, not motion curves,
sampler limits or transport timing. Physical Creative v2 feel remains unverified.

The first diagnostics push had one PowerShell 7 fixture startup timeout. Both
local PowerShell versions and both subsequent release-candidate CI installer jobs
passed without a gate change. The Windows packaging lifecycle also passed.
Ignored review output and gate logs are under `.scratch/release-alpha61/`.
