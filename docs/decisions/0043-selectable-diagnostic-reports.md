# ADR 0043: Selectable diagnostic reports

Status: Accepted on the separate review branch, 2026-10-11.

## Context

The trace export card required users to gather motion evidence, settings and
chat failures separately. A browser-built bundle would repeat backend access
and redaction decisions and could inherit the display page's message limit.

## Decision

- `GET /api/diagnostics/report` builds one ZIP on demand. The existing host
  administrator boundary applies without requiring device-control ownership.
  A dedicated admission lane allows one download at a time, independently of
  status, controller and Emergency Stop capacity.
- A `magichandy.diagnostics.v1` manifest names every file, the inclusion choices
  and collection limitations. Status is mandatory. Public settings, live/stopped
  traces and retained chat failure metadata are included by default.
- Chat content is None, Failures only, or Full retained committed history plus
  failures. Raw failed/repair output, personas with lore, prompt sets with
  captured failure prompts, and saved memories are independent opt-ins. The UI
  explains that captured prompts can themselves contain conversation and
  personalization. Portraits, audio, databases, environment dumps, account
  sessions and credentials are not included.
- Freeform persona/anatomy and voice-prompt text is excluded from settings and
  exported only through its corresponding personalization option. Closing the
  menu preserves its temporary choices and visible summary; choices do not
  change application settings.
- Every decoded string crosses a final redaction boundary for stored cloud
  credentials, common token forms, URLs and local paths. Decoding first catches
  JSON-escaped secrets and keeps replacements from damaging JSON syntax.
  Credential-store failure prevents download.
- Full history reads 25 messages per page through the chat domain, up to each
  session's captured committed head. Failures-only export never queries successful
  messages. Each session has a JSON Lines file and an index entry. The existing
  1,000-message and 24-failure retention limits are reported; pending/pruned/deleted
  content is not invented. Concurrent history changes are counted and noted,
  rather than blocking ongoing chat with a long transaction.
- The backend completes the ZIP in a private temporary file before serving it
  and removes that file afterward. Generation has a 30-second deadline and a
  64 MiB aggregate uncompressed limit. Failure returns an error, not a successful
  partial archive. No dependency, worker, model call or motion command is added.
- The UI replaces the trace card with Download / Customize in the report group.
  Its copyable status summary remains behind a disclosure. Existing trace APIs
  remain available for tooling.

## Consequences and evidence

The report describes retained evidence collected during an interval, not an
atomic backup or physical telemetry. Optional content can remain personal after
credential redaction. Nothing is uploaded automatically. Exclusion, redaction,
history completeness, access, resource bounds and actual browser downloads are
covered in the [review record](../diagnostic-report-review-2026-10-11.md).
