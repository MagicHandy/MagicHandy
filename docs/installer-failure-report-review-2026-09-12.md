# Installer failure reports — September 12, 2026

## Behavior

Every failed in-app setup job offers **Download failure report** beside the
error. The shared control appears in guided setup, TTS module updates and
Parakeet repair. Returning to setup after onboarding also surfaces the last
failed job. The prompt reads: “Please share this report with the developer to
help fix the installation failure.” All five supported locales include the new
copy. One click fetches the requested job's JSON attachment and initiates the
browser download, with an inline retry message if downloading fails.

The report contains app version/commit, operating-system architecture, Go
version, CPU thread count, detected GPU/VRAM, installation identity/device,
status, progress, timestamps, failed steps and recent terminal output. The
output tail is capped at 16 KiB before JSON encoding. The existing private
setup-result file stays capped at 64 KiB, reducing the output tail further if
JSON escaping would exceed the limit. Status polling does not include the
retained report. The normal authenticated API boundary applies without requiring
controller ownership just to download diagnostics.

The latest report is retained across retries and restarts. An older report
request never returns a newer failure: once replaced, its ID returns 404.
Pre-feature persisted failures still provide available metadata and clearly
state that original output is unavailable.

## Data handling

Reports omit settings, chat history, audio and environment dumps. Redaction
removes known configured keys, values of secret-shaped environment variables,
common credential lines/tokens, authorization values, URLs, and local paths.
URL-escaped key variants are removed too. PowerShell-wrapped quoted paths and
known credentials are handled before ordinary single-line redaction. Metadata
is redacted before truncation so shortening it does not expose part of a key.
Partial first log lines are discarded when the live output was truncated.

Only the redacted report is persisted. Downloads reapply redaction against
current keys, use a fixed attachment filename, disable caching and set
`X-Content-Type-Options: nosniff`. The report asks users to review it before
sharing. Downloading does not send anything to the developer automatically.

## Verification

- Full Go tests, race tests, vet, lint, architecture/lifecycle gates and the
  `CGO_ENABLED=0` core build pass.
- Go regressions cover useful error retention, configured/environment/encoded
  credentials, wrapped locations, native log fragments, size bounds, original
  app version preservation after restart, retry/replacement identity, legacy
  failures, and authentication.
- Component tests cover the exact one-click fetch/download, URL cleanup,
  download retry, no report for non-failed jobs, and all three failure surfaces.
  The full frontend suite has 526 passing tests in 72 files; localization and
  production build pass.
- A controlled Chatterbox CPU install in the isolated current-source app was
  blocked by an app-owned test file before Git/uv or dependency downloads. This
  exercised a real PowerShell failure through the Go manager and retained its
  diagnostics. The blocking file was removed afterward.
- Clicking the visible report button returned HTTP 200 and emitted the browser's
  `Page.downloadWillBegin` event with filename
  `magichandy-install-failure.json`. The report was inspected and wrapped local
  paths were absent. This is a controlled failure demonstration, not a
  reproduction of the original user's Chatterbox failure.

## Review app and budgets

The isolated app is left at `http://127.0.0.1:49979/#/setup/reconfigure`, displaying
the controlled failed job and report button. Voice and LLM motion remain off;
motion is routed to the simulator. It uses the available local Ollama model
`huihui_ai/granite4.1-abliterated:3b` and the review LLM generation probe.

Compared with the preceding TTS fixes: stripped app +83,968 B, main JS gzip-9
+683 B, all embedded assets +3,556 B. No dependency was added. Detailed
measurements and their limitations are in the goal scorecard.
