# Alpha.90 release review — 2026-11-26

Alpha.90 combines two changes on top of alpha.89 (`1b80d72e`):

- **PR #377** loads shared stylesheets with the app shell again. Alpha.89 had
  left Chat's Autopilot card, the persona switcher and the device connection
  choices unstyled on a first visit.
- **PR #378** makes ElevenLabs speech work with current accounts and adds a
  voice browser.

Motion, transport, Stop and installer code are unchanged.

## Gates

Both PRs passed the full CI suite on their merge refs: gofmt, vet, golangci-lint
v2.12.2, `go test`, `go test -race`, the architecture and import-boundary
tests, the frontend build and tests, the installer lifecycle, and the Windows
package. Before pushing, the same checks ran on Windows except the race suite.
The frontend suite runs the new stylesheet-ownership check and 1,149 tests.

## Stylesheet regression

The alpha.89 build and the fixed build ran side by side on simulated
transport, with fresh data directories. Headless Edge loaded `#/chat` and
`#/settings/devices/handy` directly, without visiting other routes first.

| View | alpha.89 | alpha.90 |
| --- | --- | --- |
| Chat › Autopilot | Default `<fieldset>` border inside the card; body-sized hints | One card level; compact hints |
| Chat persona switcher | "MMagicHandy" text with a bare caret | Avatar chip with name and chevron |
| Settings › Devices › The Handy › Method | Radio inputs run together as inline text | Four selectable rows with descriptions |

The `<fieldset>` has wrapped the Autopilot preferences since 2026-07-30
(`dd2cb230`), so saving or read-only state can disable them all at once. Its
reset only stopped loading in Chat when #373 made `autopilot.css` route-only.

## ElevenLabs

A review build of #378 ran on simulated transport with Gemma 4 12B QAT through
Ollama. `scripts/check-review-llm.ps1` reported `ready: true` with a real
reply. A restricted, credit-capped, 7-day review key on a free plan was used,
held only in memory and the app's key store, and never printed.

| Step | Result |
| --- | --- |
| Save the key | Accepted, default voice Jessica, model `eleven_v4_turbo` |
| Account check | `ok`, free plan, credits read, no voice or model problem |
| Turn on Speak replies (`PUT /api/voice/preferences`) | Speech worker started and loaded with no other action |
| Text-only chat message | Two-sentence reply streamed in 0.9 s |
| Spoken reply | 24 kHz PCM, 311,040 B (6.48 s), first audio 185 ms, done in 1,249 ms |
| Motion during the reply | Simulated transport idle, 0 commands |
| Voice settings and browser | Status strip, voice summary and both browser tabs rendered against the live API |

On the same free plan, the opt-in live test measured first audio per model
for one short sentence:

| Model | First audio | Request |
| --- | ---: | ---: |
| Flash v2.5 | 159 ms | 229 ms |
| Eleven v4 Turbo | 172 ms | 899 ms |
| Eleven v3 | 383 ms | 1,261 ms |
| Multilingual v2 | 463 ms | 490 ms |
| Eleven v4 | 572 ms | 1,349 ms |

The live checks also confirmed the API behavior the fix depends on:

- A Voice Library voice returns `402 paid_plan_required` on a free plan.
- A missing permission returns `401`.
- The retired default voice ID now resolves to a library voice.

## Not covered

- Physical-device acceptance. No device code changed.
- Human listening.
- Adding a Voice Library voice on a paid plan. The free plan refuses it by
  design, and the add path is covered by the mock-API tests.
- Locales other than English were checked by the localization audit, not by
  a native reader.
